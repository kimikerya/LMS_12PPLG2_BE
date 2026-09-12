package classroom

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"lms-website-be/internal/database"
	"lms-website-be/internal/middleware"
	"log"
	"net/http"
	"sort"
	"strconv"
)

type MembersInput struct {
	UserIDs []uint64 `json:"user_ids"`
	Status  string   `json:"status"`
}
type TeacherInput struct {
	TeacherID uint64  `json:"teacher_user_id"`
	SubjectID *uint64 `json:"subject_id"`
	Role      string  `json:"role"`
	Status    string  `json:"status"`
}
type membershipError string

func (e membershipError) Error() string { return string(e) }
func activeClass(ctx context.Context, q database.Querier, id uint64) error {
	var status string
	err := q.QueryRowContext(ctx, "SELECT status FROM classes WHERE id = ? FOR UPDATE", id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return membershipError("kelas tidak ditemukan")
	}
	if err != nil {
		return err
	}
	if status != "active" {
		return membershipError("kelas sudah diarsipkan")
	}
	return nil
}
func roleCheck(ctx context.Context, q database.Querier, id uint64, role string, active bool) error {
	var actual, status string
	err := q.QueryRowContext(ctx, "SELECT role, status FROM users WHERE id = ? AND deleted_at IS NULL FOR SHARE", id).Scan(&actual, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return membershipError("pengguna tidak ditemukan")
	}
	if err != nil {
		return err
	}
	if actual != role || (active && status != "active") {
		return membershipError("role atau status pengguna tidak sesuai")
	}
	return nil
}
func (s *Service) SetMembers(ctx context.Context, classID, actorID uint64, actorRole string, in MembersInput) error {
	if actorRole != "admin" {
		return membershipError("hanya admin dapat mengelola anggota")
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if classID == 0 || len(in.UserIDs) < 1 || len(in.UserIDs) > 100 || (in.Status != "active" && in.Status != "removed") {
		return membershipError("isi 1–100 user_ids dan status active/removed")
	}
	ids := append([]uint64(nil), in.UserIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, id := range ids {
		if id == 0 || (i > 0 && id == ids[i-1]) {
			return membershipError("user_ids harus unik dan lebih dari nol")
		}
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		if err := activeClass(ctx, q, classID); err != nil {
			return err
		}
		for _, id := range ids {
			if err := roleCheck(ctx, q, id, "student", in.Status == "active"); err != nil {
				return err
			}
			if in.Status == "removed" {
				if _, err := q.ExecContext(ctx, "UPDATE class_members SET status = 'removed', removed_at = UTC_TIMESTAMP() WHERE class_id = ? AND student_user_id = ?", classID, id); err != nil {
					return err
				}
			} else {
				_, err := q.ExecContext(ctx, `INSERT INTO class_members (class_id, student_user_id, joined_via) VALUES (?, ?, 'admin') ON DUPLICATE KEY UPDATE status = 'active', removed_at = NULL`, classID, id)
				if err != nil {
					return err
				}
			}
		}
		_, err := q.ExecContext(ctx, `INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id) VALUES (?, ?, 'classes', ?)`, actorID, "members_"+in.Status, classID)
		return err
	})
}
func (s *Service) SetTeacher(ctx context.Context, classID, actorID uint64, actorRole string, in TeacherInput) error {
	if actorRole != "admin" {
		return membershipError("hanya admin dapat mengelola guru kelas")
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if classID == 0 || in.TeacherID == 0 || (in.Role != "homeroom" && in.Role != "subject_teacher") || (in.Status != "active" && in.Status != "removed") {
		return membershipError("data penugasan guru tidak valid")
	}
	if in.Role == "subject_teacher" && (in.SubjectID == nil || *in.SubjectID == 0) {
		return membershipError("subject_id wajib untuk guru mata pelajaran")
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		if err := activeClass(ctx, q, classID); err != nil {
			return err
		}
		if err := roleCheck(ctx, q, in.TeacherID, "teacher", in.Status == "active"); err != nil {
			return err
		}
		if in.SubjectID != nil {
			var exists bool
			if err := q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM subjects WHERE id = ? AND is_active = TRUE)", *in.SubjectID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return membershipError("mata pelajaran tidak ditemukan atau tidak aktif")
			}
		}
		// A class row lock serializes replacement of its active homeroom teacher.
		if in.Role == "homeroom" && in.Status == "active" {
			if _, err := q.ExecContext(ctx, "UPDATE class_teachers SET status = 'removed' WHERE class_id = ? AND role = 'homeroom' AND status = 'active'", classID); err != nil {
				return err
			}
		}
		var id uint64
		err := q.QueryRowContext(ctx, `SELECT id FROM class_teachers WHERE class_id = ? AND teacher_user_id = ? AND subject_id <=> ? AND role = ? ORDER BY id LIMIT 1`, classID, in.TeacherID, in.SubjectID, in.Role).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			if in.Status == "removed" {
				return membershipError("penugasan guru tidak ditemukan")
			}
			_, err = q.ExecContext(ctx, "INSERT INTO class_teachers (class_id, teacher_user_id, subject_id, role) VALUES (?, ?, ?, ?)", classID, in.TeacherID, in.SubjectID, in.Role)
		} else if err == nil {
			_, err = q.ExecContext(ctx, "UPDATE class_teachers SET status = ? WHERE id = ?", in.Status, id)
		}
		if err != nil {
			return err
		}
		_, err = q.ExecContext(ctx, `INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id) VALUES (?, ?, 'classes', ?)`, actorID, "teacher_"+in.Status, classID)
		return err
	})
}

func (h *Handler) Members(w http.ResponseWriter, r *http.Request) { h.membership(w, r, false) }
func (h *Handler) Teacher(w http.ResponseWriter, r *http.Request) { h.membership(w, r, true) }
func (h *Handler) membership(w http.ResponseWriter, r *http.Request, teacher bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, 400, "id kelas tidak valid")
		return
	}
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "admin" {
		writeError(w, 403, "akses khusus admin")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if teacher {
		var in TeacherInput
		if d.Decode(&in) != nil || d.Decode(&struct{}{}) != io.EOF {
			writeError(w, 400, "kirim satu objek JSON yang valid")
			return
		}
		err = h.service.SetTeacher(r.Context(), id, claims.UserID, claims.Role, in)
	} else {
		var in MembersInput
		if d.Decode(&in) != nil || d.Decode(&struct{}{}) != io.EOF {
			writeError(w, 400, "kirim satu objek JSON yang valid")
			return
		}
		err = h.service.SetMembers(r.Context(), id, claims.UserID, claims.Role, in)
	}
	if err != nil {
		var inputErr membershipError
		if errors.As(err, &inputErr) {
			writeError(w, 422, err.Error())
		} else {
			log.Printf("membership: %v", err)
			writeError(w, 500, "operasi anggota belum berhasil; silakan coba kembali")
		}
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}
