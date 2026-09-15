package learning

import (
	"context"
	"database/sql"
	"errors"
	"lms-website-be/internal/database"
	"net/http"
	"strings"
)

func (s *Service) DeleteContent(ctx context.Context, a Actor, kind string, id uint64) error {
	if a.Role != "teacher" {
		return forbidden
	}
	if kind != "materials" && kind != "assignments" && kind != "assessments" {
		return notFound
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		var owner uint64
		err := q.QueryRowContext(ctx, "SELECT teacher_user_id FROM "+kind+" WHERE id=? AND deleted_at IS NULL FOR UPDATE", id).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound
		}
		if err != nil {
			return err
		}
		if owner != a.ID {
			return forbidden
		}
		if _, err = q.ExecContext(ctx, "UPDATE "+kind+" SET deleted_at=UTC_TIMESTAMP() WHERE id=?", id); err != nil {
			return err
		}
		return audit(ctx, q, a, "delete", kind, id)
	})
}
func (h *Handler) DeleteContent(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err := h.service.DeleteContent(r.Context(), actor(r), kind, id); err != nil {
			respondError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"deleted": true})
	}
}

func (s *Service) EditMaterial(ctx context.Context, a Actor, id uint64, in MaterialInput) error {
	if a.Role != "teacher" {
		return forbidden
	}
	if !validTitle(in.Title) || !validURL(in.URL) || in.URL == nil || in.FilePath != nil {
		return invalid("Isi judul dan tautan materi yang valid.")
	}
	if in.Description != nil && len(*in.Description) > 20000 {
		return invalid("Deskripsi terlalu panjang.")
	}
	if in.MaterialType != "link" && in.MaterialType != "video" && in.MaterialType != "pdf" && in.MaterialType != "document" {
		return invalid("Jenis materi tidak valid.")
	}
	if in.MeetingNo != nil && *in.MeetingNo == 0 {
		return invalid("Pertemuan minimal 1.")
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		var owner, cid uint64
		var sid *uint64
		err := q.QueryRowContext(ctx, "SELECT teacher_user_id,class_id,subject_id FROM materials WHERE id=? AND deleted_at IS NULL FOR UPDATE", id).Scan(&owner, &cid, &sid)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound
		}
		if err != nil {
			return err
		}
		if owner != a.ID {
			return forbidden
		}
		if err = requireTeaching(ctx, q, a, cid, sid); err != nil {
			return err
		}
		if _, err = q.ExecContext(ctx, "UPDATE materials SET title=?,description=?,material_type=?,meeting_no=?,url=? WHERE id=?", strings.TrimSpace(in.Title), in.Description, in.MaterialType, in.MeetingNo, in.URL, id); err != nil {
			return err
		}
		return audit(ctx, q, a, "update", "materials", id)
	})
}
func (h *Handler) EditMaterial(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in MaterialInput
	if !decode(w, r, &in) {
		return
	}
	if err := h.service.EditMaterial(r.Context(), actor(r), id, in); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

type assignmentEdit struct {
	assignmentPackage
	RetainedIDs []uint64 `json:"retained_ids"`
}

func (s *Service) EditAssignmentPackage(ctx context.Context, a Actor, id uint64, in assignmentEdit, files []Attachment) error {
	if a.Role != "teacher" {
		return forbidden
	}
	if !validTitle(in.Title) || strings.TrimSpace(in.Instructions) == "" || len(in.Instructions) > 20000 || in.DueAt.IsZero() || in.CloseAt != nil && in.CloseAt.Before(in.DueAt) {
		return invalid("Periksa judul, petunjuk, dan jadwal tugas.")
	}
	if in.MaxPoints == nil || !validPoints(*in.MaxPoints) || *in.MaxPoints == 0 {
		return invalid("Nilai maksimal tidak valid.")
	}
	if len(in.Links) > 10 || len(in.RetainedIDs)+len(files) > 5 {
		return invalid("Maksimal 5 file dan 10 tautan.")
	}
	for _, v := range in.Links {
		if !validURL(&v.URL) || strings.TrimSpace(v.Name) == "" || len(v.Name) > 240 {
			return invalid("Periksa judul dan alamat tautan.")
		}
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		v, err := assignmentForUpdate(ctx, q, id)
		if err != nil {
			return err
		}
		if v.Owner != a.ID {
			return forbidden
		}
		if err = requireTeaching(ctx, q, a, v.ClassID, v.SubjectID); err != nil {
			return err
		}
		var count int
		if err = q.QueryRowContext(ctx, "SELECT COUNT(*) FROM assignment_submissions WHERE assignment_id=?", id).Scan(&count); err != nil {
			return err
		}
		if count > 0 && *in.MaxPoints != v.MaxPoints {
			return conflict("Nilai maksimal dikunci karena sudah ada jawaban siswa.")
		}
		keep := map[uint64]bool{}
		for _, fid := range in.RetainedIDs {
			if keep[fid] {
				return invalid("Lampiran berulang.")
			}
			keep[fid] = true
			var stored string
			if err = q.QueryRowContext(ctx, "SELECT file_url FROM assignment_attachments WHERE id=? AND assignment_id=?", fid, id).Scan(&stored); errors.Is(err, sql.ErrNoRows) {
				return invalid("Lampiran bukan milik tugas ini.")
			} else if err != nil {
				return err
			}
			if !validStoredKey(stored) {
				return invalid("Lampiran bukan file unggahan.")
			}
		}
		rows, err := q.QueryContext(ctx, "SELECT id FROM assignment_attachments WHERE assignment_id=?", id)
		if err != nil {
			return err
		}
		var remove []uint64
		for rows.Next() {
			var fid uint64
			if err = rows.Scan(&fid); err != nil {
				rows.Close()
				return err
			}
			if !keep[fid] {
				remove = append(remove, fid)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, fid := range remove {
			if _, err = q.ExecContext(ctx, "DELETE FROM assignment_attachments WHERE id=? AND assignment_id=?", fid, id); err != nil {
				return err
			}
		}
		for _, file := range append(files, in.Links...) {
			if _, err = q.ExecContext(ctx, "INSERT INTO assignment_attachments (assignment_id,file_name,file_url) VALUES (?,?,?)", id, file.Name, file.URL); err != nil {
				return err
			}
		}
		// Retain replaced file bytes for recovery; their old download IDs are no longer accessible.
		if _, err = q.ExecContext(ctx, "UPDATE assignments SET title=?,instructions=?,due_at=?,close_at=?,allow_late=?,max_points=? WHERE id=?", strings.TrimSpace(in.Title), in.Instructions, in.DueAt.UTC(), utcTime(in.CloseAt), in.AllowLate, in.MaxPoints, id); err != nil {
			return err
		}
		if in.Publish && v.Status == "draft" {
			if _, err = q.ExecContext(ctx, "UPDATE assignments SET status='published',published_at=UTC_TIMESTAMP() WHERE id=?", id); err != nil {
				return err
			}
		}
		return audit(ctx, q, a, "update", "assignments", id)
	})
}
func (h *Handler) EditAssignmentUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	root := uploadRoot()
	files, err := receiveUploads(w, r, root)
	if err != nil {
		respondError(w, err)
		return
	}
	var in assignmentEdit
	if err = multipartMetadata(r, &in); err == nil {
		err = h.service.EditAssignmentPackage(r.Context(), actor(r), id, in, files)
	}
	if err != nil {
		removeUploads(root, files)
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}
