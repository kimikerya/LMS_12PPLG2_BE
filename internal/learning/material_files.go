package learning

import (
	"context"
	"database/sql"
	"errors"
	"lms-website-be/internal/database"
	"net/http"
	"strings"
	"time"
)

type materialPackage struct {
	MaterialInput
	Publish     bool     `json:"publish"`
	RetainedIDs []uint64 `json:"retained_ids"`
}

func (s *Service) SaveMaterialPackage(ctx context.Context, a Actor, id uint64, in materialPackage, files []Attachment) (uint64, error) {
	if a.Role != "teacher" {
		return 0, forbidden
	}
	in.Title = strings.TrimSpace(in.Title)
	if !validTitle(in.Title) || in.FilePath != nil || len(files)+len(in.RetainedIDs) > 5 {
		return 0, invalid("Isi judul dan pilih maksimal 5 file materi.")
	}
	if in.Description != nil && len(*in.Description) > 20000 {
		return 0, invalid("Deskripsi maksimal 20000 karakter.")
	}
	if in.URL != nil {
		v := strings.TrimSpace(*in.URL)
		if v == "" {
			in.URL = nil
		} else {
			in.URL = &v
			if !validURL(in.URL) {
				return 0, invalid("Gunakan tautan HTTP/HTTPS yang valid.")
			}
		}
	}
	if in.URL == nil && len(files)+len(in.RetainedIDs) == 0 {
		return 0, invalid("Unggah file materi atau isi tautan sumber.")
	}
	if in.MaterialType != "link" && in.MaterialType != "video" && in.MaterialType != "pdf" && in.MaterialType != "document" {
		return 0, invalid("Jenis materi tidak valid.")
	}
	if in.MeetingNo != nil && *in.MeetingNo == 0 {
		return 0, invalid("Pertemuan minimal 1.")
	}
	err := s.repository.transact(ctx, func(q database.Querier) error {
		if id != 0 {
			var owner uint64
			var status string
			err := q.QueryRowContext(ctx, "SELECT teacher_user_id,class_id,subject_id,status FROM materials WHERE id=? AND deleted_at IS NULL FOR UPDATE", id).Scan(&owner, &in.ClassID, &in.SubjectID, &status)
			if errors.Is(err, sql.ErrNoRows) {
				return notFound
			}
			if err != nil {
				return err
			}
			if owner != a.ID {
				return forbidden
			}
			if in.Publish && status == "closed" {
				return conflict("Materi yang ditutup tidak dapat diterbitkan kembali.")
			}
		} else if len(in.RetainedIDs) > 0 {
			return invalid("Lampiran lama tidak berlaku untuk materi baru.")
		}
		if err := requireTeaching(ctx, q, a, in.ClassID, in.SubjectID); err != nil {
			return err
		}
		if id == 0 {
			var err error
			id, err = insertID(ctx, q, `INSERT INTO materials(class_id,teacher_user_id,subject_id,title,description,material_type,meeting_no,url) VALUES(?,?,?,?,?,?,?,?)`, in.ClassID, a.ID, in.SubjectID, in.Title, in.Description, in.MaterialType, in.MeetingNo, in.URL)
			if err != nil {
				return err
			}
		} else {
			if _, err := q.ExecContext(ctx, `UPDATE materials SET title=?,description=?,material_type=?,meeting_no=?,url=? WHERE id=?`, in.Title, in.Description, in.MaterialType, in.MeetingNo, in.URL, id); err != nil {
				return err
			}
		}
		rows, err := q.QueryContext(ctx, "SELECT id FROM material_attachments WHERE material_id=?", id)
		if err != nil {
			return err
		}
		existing := map[uint64]bool{}
		for rows.Next() {
			var v uint64
			if err = rows.Scan(&v); err != nil {
				rows.Close()
				return err
			}
			existing[v] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		keep := map[uint64]bool{}
		for _, v := range in.RetainedIDs {
			if !existing[v] || keep[v] {
				return invalid("Lampiran materi tidak valid.")
			}
			keep[v] = true
		}
		for v := range existing {
			if !keep[v] {
				if _, err = q.ExecContext(ctx, "DELETE FROM material_attachments WHERE id=? AND material_id=?", v, id); err != nil {
					return err
				}
			}
		}
		for _, v := range files {
			if !validStoredKey(v.URL) {
				return invalid("File materi tidak valid.")
			}
			if _, err = q.ExecContext(ctx, "INSERT INTO material_attachments(material_id,file_name,file_url) VALUES(?,?,?)", id, v.Name, v.URL); err != nil {
				return err
			}
		}
		if in.Publish {
			if _, err = q.ExecContext(ctx, "UPDATE materials SET status='published',published_at=COALESCE(published_at,UTC_TIMESTAMP()) WHERE id=?", id); err != nil {
				return err
			}
		}
		return audit(ctx, q, a, "save_material", "materials", id)
	})
	return id, err
}

func (h *Handler) UploadMaterial(w http.ResponseWriter, r *http.Request) {
	var id uint64
	if r.PathValue("id") != "" {
		var ok bool
		id, ok = pathID(w, r)
		if !ok {
			return
		}
	}
	root := uploadRoot()
	files, err := receiveUploads(w, r, root)
	if err != nil {
		respondError(w, err)
		return
	}
	var in materialPackage
	if err = multipartMetadata(r, &in); err == nil {
		id, err = h.service.SaveMaterialPackage(r.Context(), actor(r), id, in, files)
	}
	if err != nil {
		removeUploads(root, files)
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

func (r *Repository) MaterialAttachments(ctx context.Context, id uint64) ([]Attachment, error) {
	rows, err := r.q.QueryContext(ctx, "SELECT id,file_name,file_url FROM material_attachments WHERE material_id=? ORDER BY id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Attachment{}
	for rows.Next() {
		var v Attachment
		if err = rows.Scan(&v.ID, &v.Name, &v.URL); err != nil {
			return nil, err
		}
		items = append(items, attachmentLink(v, "material"))
	}
	return items, rows.Err()
}

type MaterialReader struct {
	ID     uint64     `json:"id"`
	Name   string     `json:"name"`
	NIS    *string    `json:"nis"`
	Opened *time.Time `json:"opened_at"`
}

func (s *Service) MaterialReaders(ctx context.Context, a Actor, id uint64) ([]MaterialReader, error) {
	if a.Role != "teacher" && !monitoring(a.Role) {
		return nil, forbidden
	}
	visible, err := s.List(ctx, a, "materials", Filter{Limit: 1}, id)
	if err != nil {
		return nil, err
	}
	if len(visible) == 0 {
		return nil, notFound
	}
	rows, err := s.repository.q.QueryContext(ctx, `SELECT u.id,u.full_name,p.nis,ma.first_opened_at FROM class_members cm JOIN users u ON u.id=cm.student_user_id LEFT JOIN student_profiles p ON p.user_id=u.id LEFT JOIN material_access ma ON ma.student_user_id=u.id AND ma.material_id=? WHERE cm.class_id=? AND cm.status='active' AND u.deleted_at IS NULL ORDER BY u.full_name,u.id`, id, visible[0].ClassID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []MaterialReader{}
	for rows.Next() {
		var v MaterialReader
		if err = rows.Scan(&v.ID, &v.Name, &v.NIS, &v.Opened); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (h *Handler) MaterialReaders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	items, err := h.service.MaterialReaders(r.Context(), actor(r), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}
