package learning

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/go-sql-driver/mysql"
	"io"
	"lms-website-be/internal/middleware"
	"log"
	"net/http"
	"strconv"
)

type Handler struct{ service *Service }

func NewHandler(db *sql.DB) *Handler        { return NewServiceHandler(NewService(NewRepository(db))) }
func NewServiceHandler(s *Service) *Handler { return &Handler{service: s} }

func actor(r *http.Request) Actor {
	c, _ := middleware.ClaimsFromContext(r.Context())
	return Actor{ID: c.UserID, Role: c.Role}
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	return decodeLimit(w, r, value, 128*1024)
}
func decodeLimit(w http.ResponseWriter, r *http.Request, value any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		respondError(w, &Problem{400, "JSON tidak valid, kolom tidak dikenal, atau body terlalu besar"})
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		respondError(w, &Problem{400, "kirim tepat satu objek JSON"})
		return false
	}
	return true
}
func pathID(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		respondError(w, &Problem{400, "id tidak valid"})
		return 0, false
	}
	return id, true
}
func filter(r *http.Request) (Filter, error) {
	f := Filter{Limit: 50}
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		f.Limit, err = strconv.Atoi(raw)
		if err != nil || f.Limit < 1 {
			return f, invalid("limit tidak valid")
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		f.Offset, err = strconv.Atoi(raw)
		if err != nil {
			return f, invalid("offset tidak valid")
		}
	}
	if raw := r.URL.Query().Get("class_id"); raw != "" {
		f.ClassID, err = strconv.ParseUint(raw, 10, 64)
		if err != nil || f.ClassID == 0 {
			return f, invalid("class_id tidak valid")
		}
	}
	if raw := r.URL.Query().Get("subject_id"); raw != "" {
		f.SubjectID, err = strconv.ParseUint(raw, 10, 64)
		if err != nil || f.SubjectID == 0 {
			return f, invalid("subject_id tidak valid")
		}
	}
	return f, nil
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request, kind string) {
	f, err := filter(r)
	if err != nil {
		respondError(w, err)
		return
	}
	items, err := h.service.List(r.Context(), actor(r), kind, f, 0)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": items, "limit": f.Limit, "offset": f.Offset})
}
func (h *Handler) Detail(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		items, err := h.service.List(r.Context(), actor(r), kind, Filter{Limit: 1}, id)
		if err != nil {
			respondError(w, err)
			return
		}
		if len(items) == 0 {
			respondError(w, notFound)
			return
		}
		writeJSON(w, 200, items[0])
	}
}
func (h *Handler) ListMaterials(w http.ResponseWriter, r *http.Request) { h.list(w, r, "materials") }
func (h *Handler) ListAssignments(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, "assignments")
}
func (h *Handler) ListAssessments(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, "assessments")
}
func created(w http.ResponseWriter, a Actor, id uint64, err error) {
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "teacher_user_id": a.ID, "status": "draft"})
}
func (h *Handler) CreateMaterial(w http.ResponseWriter, r *http.Request) {
	var in MaterialInput
	if !decode(w, r, &in) {
		return
	}
	a := actor(r)
	id, err := h.service.CreateMaterial(r.Context(), a, in)
	created(w, a, id, err)
}
func (h *Handler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	var in AssignmentInput
	if !decode(w, r, &in) {
		return
	}
	a := actor(r)
	id, err := h.service.CreateAssignment(r.Context(), a, in)
	created(w, a, id, err)
}
func (h *Handler) CreateAssessment(w http.ResponseWriter, r *http.Request) {
	var in AssessmentInput
	if !decode(w, r, &in) {
		return
	}
	a := actor(r)
	id, err := h.service.CreateAssessment(r.Context(), a, in)
	created(w, a, id, err)
}
func (h *Handler) Publish(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err := h.service.Publish(r.Context(), actor(r), kind, id); err != nil {
			respondError(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"status": "published"})
	}
}
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in SubmitInput
	if !decode(w, r, &in) {
		return
	}
	subID, err := h.service.Submit(r.Context(), actor(r), id, in)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": subID, "assignment_id": id})
}
func (h *Handler) Submissions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	items, err := h.service.Submissions(r.Context(), actor(r), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}
func (h *Handler) Grade(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in GradeInput
	if !decode(w, r, &in) {
		return
	}
	if err := h.service.Grade(r.Context(), actor(r), id, in); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "graded"})
}
func (h *Handler) Release(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.service.Release(r.Context(), actor(r), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "released"})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func respondError(w http.ResponseWriter, err error) {
	var p *Problem
	if errors.As(err, &p) {
		writeJSON(w, p.Status, map[string]string{"error": p.Message})
		return
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		writeJSON(w, 409, map[string]string{"error": "data sudah ada"})
		return
	}
	log.Printf("learning: %v", err)
	writeJSON(w, 500, map[string]string{"error": "operasi belum berhasil; silakan coba kembali"})
}
