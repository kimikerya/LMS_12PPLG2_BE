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
	"strconv"
	"strings"
	"unicode/utf8"
)

type Option struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}
type AcademicOptions struct {
	Years    []Option `json:"years"`
	Levels   []Option `json:"levels"`
	Majors   []Option `json:"majors"`
	Subjects []Option `json:"subjects"`
}

func (s *Service) options(ctx context.Context) (AcademicOptions, error) {
	out := AcademicOptions{}
	for _, entry := range []struct {
		query string
		dest  *[]Option
	}{
		{"SELECT id,name FROM academic_years ORDER BY is_active DESC,name DESC", &out.Years},
		{"SELECT id,name FROM education_levels ORDER BY name", &out.Levels},
		{"SELECT id,name FROM majors WHERE is_active=TRUE ORDER BY name", &out.Majors},
		{"SELECT id,name FROM subjects WHERE is_active=TRUE ORDER BY name", &out.Subjects},
	} {
		*entry.dest = []Option{}
		rows, err := s.repository.db.QueryContext(ctx, entry.query)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var o Option
			if err = rows.Scan(&o.ID, &o.Name); err != nil {
				rows.Close()
				return out, err
			}
			*entry.dest = append(*entry.dest, o)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func (h *Handler) Options(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.options(r.Context())
	if err != nil {
		classError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

type Member struct {
	ID       uint64  `json:"id"`
	LoginID  string  `json:"login_id"`
	FullName string  `json:"full_name"`
	Status   string  `json:"status"`
	NIS      *string `json:"nis"`
}
type Teacher struct {
	ID          uint64  `json:"id"`
	TeacherID   uint64  `json:"teacher_user_id"`
	FullName    string  `json:"full_name"`
	Role        string  `json:"role"`
	SubjectID   *uint64 `json:"subject_id"`
	SubjectName *string `json:"subject_name"`
	Status      string  `json:"status"`
}
type Announcement struct {
	ID        uint64 `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
}
type ClassDetail struct {
	Class
	MemberCount   int            `json:"member_count"`
	Year          string         `json:"year"`
	Level         string         `json:"level"`
	Major         *string        `json:"major"`
	Members       []Member       `json:"members"`
	Teachers      []Teacher      `json:"teachers"`
	Announcements []Announcement `json:"announcements"`
}

func (s *Service) detail(ctx context.Context, id uint64) (ClassDetail, error) {
	items, err := s.workspace(ctx, 0, "admin", id, false)
	if err != nil {
		return ClassDetail{}, err
	}
	return items[0], nil
}
func classID(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, 400, "ID kelas tidak valid.")
		return 0, false
	}
	return id, true
}
func classError(w http.ResponseWriter, err error) {
	var invalid membershipError
	var denied accessError
	switch {
	case errors.As(err, &denied):
		writeError(w, 403, denied.Error())
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, 404, "Kelas tidak ditemukan.")
	case errors.As(err, &invalid):
		writeError(w, 422, err.Error())
	default:
		log.Printf("class management: %v", err)
		writeError(w, 500, "Data kelas belum dapat diproses. Silakan coba kembali.")
	}
}
func classDecode(w http.ResponseWriter, r *http.Request, in any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(in) != nil || d.Decode(&struct{}{}) != io.EOF {
		writeError(w, 400, "Kirim satu objek JSON yang valid.")
		return false
	}
	return true
}
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	id, ok := classID(w, r)
	if !ok {
		return
	}
	claims, authenticated := middleware.ClaimsFromContext(r.Context())
	if !authenticated {
		writeError(w, 401, "Masuk ke portal dahulu.")
		return
	}
	if err := classAccess(r.Context(), h.service.repository.db, id, claims.UserID, claims.Role, false); err != nil {
		classError(w, err)
		return
	}
	items, err := h.service.workspace(r.Context(), claims.UserID, claims.Role, id, false)
	if err != nil {
		classError(w, err)
		return
	}
	out := items[0]
	if claims.Role == "student" {
		out.Members = []Member{}
	}
	writeJSON(w, 200, out)
}
func validateClass(ctx context.Context, q database.Querier, in *CreateInput) error {
	if in.MajorID == nil || *in.MajorID == 0 {
		return membershipError("Jurusan wajib dipilih. Hanya deskripsi dan ruang kelas yang boleh kosong.")
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 100 || in.GradeLevel < 1 || in.GradeLevel > 12 {
		return membershipError("Nama kelas wajib diisi (maksimal 100 karakter), tingkat kelas 1–12.")
	}
	if in.Room != nil && utf8.RuneCountInString(*in.Room) > 50 {
		return membershipError("Ruang maksimal 50 karakter.")
	}
	if in.Description != nil && utf8.RuneCountInString(*in.Description) > 5000 {
		return membershipError("Deskripsi maksimal 5000 karakter.")
	}
	for _, ref := range []struct {
		query string
		id    uint64
	}{{"SELECT COUNT(*) FROM academic_years WHERE id=?", in.AcademicYearID}, {"SELECT COUNT(*) FROM education_levels WHERE id=?", in.EducationLevelID}} {
		var count int
		if err := q.QueryRowContext(ctx, ref.query, ref.id).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return membershipError("Pilih tahun ajaran dan jenjang yang tersedia.")
		}
	}
	if in.MajorID != nil {
		var count int
		if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM majors WHERE id=? AND is_active=TRUE", *in.MajorID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return membershipError("Jurusan tidak tersedia.")
		}
	}
	return nil
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := classID(w, r)
	if !ok {
		return
	}
	var in CreateInput
	if !classDecode(w, r, &in) {
		return
	}
	err := h.service.repository.transact(r.Context(), func(q database.Querier) error {
		if err := activeClass(r.Context(), q, id); err != nil {
			return err
		}
		if err := validateClass(r.Context(), q, &in); err != nil {
			return err
		}
		_, err := q.ExecContext(r.Context(), "UPDATE classes SET academic_year_id=?,education_level_id=?,major_id=?,grade_level=?,title=?,description=?,room=? WHERE id=?", in.AcademicYearID, in.EducationLevelID, in.MajorID, in.GradeLevel, in.Title, in.Description, in.Room, id)
		return err
	})
	if err != nil {
		classError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

type AnnouncementInput struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (s *Service) announce(ctx context.Context, id, actorID uint64, in AnnouncementInput) error {
	in.Title = strings.TrimSpace(in.Title)
	in.Content = strings.TrimSpace(in.Content)
	if in.Title == "" || in.Content == "" || utf8.RuneCountInString(in.Title) > 200 || utf8.RuneCountInString(in.Content) > 10000 {
		return membershipError("Judul (maksimal 200 karakter) dan isi (maksimal 10000 karakter) wajib diisi.")
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		if err := activeClass(ctx, q, id); err != nil {
			return err
		}
		_, err := q.ExecContext(ctx, "INSERT INTO announcements (class_id,author_user_id,title,content,created_at,updated_at) VALUES (?,?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())", id, actorID, in.Title, in.Content)
		return err
	})
}
func (h *Handler) Announce(w http.ResponseWriter, r *http.Request) {
	id, ok := classID(w, r)
	if !ok {
		return
	}
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "admin" {
		writeError(w, 403, "Akses khusus admin.")
		return
	}
	var in AnnouncementInput
	if !classDecode(w, r, &in) {
		return
	}
	if err := h.service.announce(r.Context(), id, claims.UserID, in); err != nil {
		classError(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"status": "created"})
}
