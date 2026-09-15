package learning

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"lms-website-be/internal/database"
	"net/http"
	"strings"
	"time"
)

type AssessmentOption struct {
	ID      uint64 `json:"id,omitempty"`
	Text    string `json:"text"`
	Correct *bool  `json:"correct,omitempty"`
}
type AssessmentQuestion struct {
	ID      uint64             `json:"id,omitempty"`
	Type    string             `json:"type"`
	Text    string             `json:"text"`
	Points  float64            `json:"points"`
	Options []AssessmentOption `json:"options"`
}
type AssessmentDraft struct {
	AssessmentInput
	HasAttempts bool                 `json:"has_attempts,omitempty"`
	ID          uint64               `json:"id,omitempty"`
	Status      string               `json:"status,omitempty"`
	ClassIDs    []uint64             `json:"class_ids"`
	Questions   []AssessmentQuestion `json:"questions"`
}

func validateAssessment(in AssessmentDraft, publish bool) error {
	if !validTitle(in.Title) || (in.AssessmentType != "quiz" && in.AssessmentType != "online_exam") {
		return invalid("Judul dan jenis asesmen wajib diisi.")
	}
	if in.Description != nil && len(*in.Description) > 20000 || in.Instructions != nil && len(*in.Instructions) > 20000 {
		return invalid("Deskripsi atau petunjuk terlalu panjang.")
	}
	if in.SubjectID == nil || *in.SubjectID == 0 || len(in.ClassIDs) == 0 || len(in.ClassIDs) > 50 {
		return invalid("Pilih mata pelajaran dan minimal satu kelas ajar.")
	}
	if in.DurationMinutes != nil && (*in.DurationMinutes == 0 || *in.DurationMinutes > 480) {
		return invalid("Durasi harus antara 1 dan 480 menit.")
	}
	if in.StartAt != nil && in.EndAt != nil && !in.EndAt.After(*in.StartAt) {
		return invalid("Waktu selesai harus setelah waktu mulai.")
	}
	if len(in.Questions) > 50 {
		return invalid("Maksimal 50 soal per asesmen.")
	}
	if publish && (in.DurationMinutes == nil || in.StartAt == nil || in.EndAt == nil || len(in.Questions) == 0) {
		return invalid("Lengkapi durasi, jadwal, dan minimal satu soal sebelum menerbitkan.")
	}
	if publish && !in.EndAt.After(time.Now()) {
		return invalid("Jadwal selesai sudah lewat. Perbarui jadwal terlebih dahulu.")
	}
	total := 0.0
	for i, q := range in.Questions {
		if len(q.Text) > 10000 || !validPoints(q.Points) || q.Points == 0 {
			return invalid(fmt.Sprintf("Periksa teks dan bobot soal %d (maksimal dua desimal).", i+1))
		}
		total += q.Points
		if publish && strings.TrimSpace(q.Text) == "" {
			return invalid(fmt.Sprintf("Isi pertanyaan pada soal %d.", i+1))
		}
		switch q.Type {
		case "essay":
			if len(q.Options) != 0 {
				return invalid("Soal esai tidak memiliki pilihan jawaban.")
			}
		case "single_choice", "multiple_choice":
			if len(q.Options) < 2 || len(q.Options) > 6 {
				return invalid("Pilihan ganda membutuhkan 2 sampai 6 pilihan.")
			}
			count := 0
			for _, o := range q.Options {
				if len(o.Text) > 2000 || publish && strings.TrimSpace(o.Text) == "" {
					return invalid(fmt.Sprintf("Lengkapi pilihan jawaban soal %d.", i+1))
				}
				if o.Correct != nil && *o.Correct {
					count++
				}
			}
			if publish && (count == 0 || q.Type == "single_choice" && count != 1) {
				return invalid(fmt.Sprintf("Periksa kunci jawaban soal %d.", i+1))
			}
		default:
			return invalid("Jenis soal tidak didukung.")
		}
	}
	if total > 9999.99 {
		return invalid("Total bobot maksimal 9999,99.")
	}
	return nil
}

func readAssessment(ctx context.Context, q database.Querier, id uint64) (AssessmentDraft, uint64, error) {
	in := AssessmentDraft{ID: id, ClassIDs: []uint64{}, Questions: []AssessmentQuestion{}}
	var owner uint64
	err := q.QueryRowContext(ctx, `SELECT teacher_user_id,subject_id,title,assessment_type,description,instructions,duration_minutes,start_at,end_at,status FROM assessments WHERE id=? AND deleted_at IS NULL`, id).Scan(&owner, &in.SubjectID, &in.Title, &in.AssessmentType, &in.Description, &in.Instructions, &in.DurationMinutes, &in.StartAt, &in.EndAt, &in.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return in, 0, notFound
	}
	if err != nil {
		return in, 0, err
	}
	rows, err := q.QueryContext(ctx, "SELECT class_id FROM assessment_targets WHERE assessment_id=? ORDER BY class_id", id)
	if err != nil {
		return in, 0, err
	}
	for rows.Next() {
		var cid uint64
		if err = rows.Scan(&cid); err != nil {
			rows.Close()
			return in, 0, err
		}
		in.ClassIDs = append(in.ClassIDs, cid)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return in, 0, err
	}
	rows, err = q.QueryContext(ctx, "SELECT id,question_type,question_text,points FROM assessment_questions WHERE assessment_id=? ORDER BY order_no", id)
	if err != nil {
		return in, 0, err
	}
	for rows.Next() {
		v := AssessmentQuestion{Options: []AssessmentOption{}}
		if err = rows.Scan(&v.ID, &v.Type, &v.Text, &v.Points); err != nil {
			rows.Close()
			return in, 0, err
		}
		in.Questions = append(in.Questions, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return in, 0, err
	}
	indices := make(map[uint64]int, len(in.Questions))
	for i := range in.Questions {
		indices[in.Questions[i].ID] = i
	}
	rows, err = q.QueryContext(ctx, `SELECT o.question_id,o.id,o.option_text,o.is_correct FROM question_options o JOIN assessment_questions aq ON aq.id=o.question_id WHERE aq.assessment_id=? ORDER BY aq.order_no,o.order_no`, id)
	if err != nil {
		return in, 0, err
	}
	for rows.Next() {
		var questionID uint64
		var o AssessmentOption
		if err = rows.Scan(&questionID, &o.ID, &o.Text, &o.Correct); err != nil {
			rows.Close()
			return in, 0, err
		}
		if i, ok := indices[questionID]; ok {
			in.Questions[i].Options = append(in.Questions[i].Options, o)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return in, 0, err
	}
	return in, owner, nil
}
func lockAssessment(ctx context.Context, q database.Querier, id uint64) (uint64, string, error) {
	var owner uint64
	var status string
	err := q.QueryRowContext(ctx, "SELECT teacher_user_id,status FROM assessments WHERE id=? AND deleted_at IS NULL FOR UPDATE", id).Scan(&owner, &status)
	if errors.Is(err, sql.ErrNoRows) {
		err = notFound
	}
	return owner, status, err
}
func (s *Service) AssessmentEditor(ctx context.Context, a Actor, id uint64) (AssessmentDraft, error) {
	if a.Role != "teacher" {
		return AssessmentDraft{}, forbidden
	}
	in, owner, err := readAssessment(ctx, s.repository.q, id)
	if err != nil {
		return in, err
	}
	if owner != a.ID {
		return AssessmentDraft{}, forbidden
	}
	err = s.repository.q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM assessment_attempts WHERE assessment_id=?)", id).Scan(&in.HasAttempts)
	return in, err
}
func (s *Service) SaveAssessment(ctx context.Context, a Actor, id uint64, in AssessmentDraft) (uint64, error) {
	if a.Role != "teacher" {
		return 0, forbidden
	}
	in.Title = strings.TrimSpace(in.Title)
	if err := validateAssessment(in, false); err != nil {
		return 0, err
	}
	err := s.repository.transact(ctx, func(q database.Querier) error {
		if id != 0 {
			owner, status, err := lockAssessment(ctx, q, id)
			if err != nil {
				return err
			}
			if owner != a.ID {
				return forbidden
			}
			var n int
			if err = q.QueryRowContext(ctx, "SELECT COUNT(*) FROM assessment_attempts WHERE assessment_id=?", id).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				old, _, err := readAssessment(ctx, q, id)
				if err != nil {
					return err
				}
				before := old
				after := in
				before.Title = ""
				before.Description = nil
				before.Instructions = nil
				before.ID = 0
				before.Status = ""
				before.HasAttempts = false
				after.Title = ""
				after.Description = nil
				after.Instructions = nil
				after.ID = 0
				after.Status = ""
				after.HasAttempts = false
				before.StartAt = utcTime(before.StartAt)
				before.EndAt = utcTime(before.EndAt)
				after.StartAt = utcTime(after.StartAt)
				after.EndAt = utcTime(after.EndAt)
				b, _ := json.Marshal(before)
				c, _ := json.Marshal(after)
				if string(b) != string(c) {
					return conflict("Sudah ada pengerjaan siswa. Hanya judul, deskripsi, dan petunjuk yang dapat diubah.")
				}
				for _, cid := range old.ClassIDs {
					if err = requireTeaching(ctx, q, a, cid, old.SubjectID); err != nil {
						return err
					}
				}
				if _, err = q.ExecContext(ctx, "UPDATE assessments SET title=?,description=?,instructions=? WHERE id=?", in.Title, in.Description, in.Instructions, id); err != nil {
					return err
				}
				return audit(ctx, q, a, "update", "assessments", id)
			}
			if status != "draft" {
				if err = validateAssessment(in, true); err != nil {
					return err
				}
			}
		}
		seen := map[uint64]bool{}
		for _, cid := range in.ClassIDs {
			if seen[cid] {
				return invalid("Kelas tidak boleh berulang.")
			}
			seen[cid] = true
			if err := requireTeaching(ctx, q, a, cid, in.SubjectID); err != nil {
				return err
			}
		}
		var subjectActive int
		if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM subjects WHERE id=? AND is_active=TRUE", in.SubjectID).Scan(&subjectActive); err != nil {
			return err
		}
		if subjectActive == 0 {
			return invalid("Mata pelajaran tidak aktif.")
		}
		var err error
		if id == 0 {
			id, err = insertID(ctx, q, `INSERT INTO assessments (teacher_user_id,subject_id,title,assessment_type,description,instructions,duration_minutes,start_at,end_at) VALUES (?,?,?,?,?,?,?,?,?)`, a.ID, in.SubjectID, in.Title, in.AssessmentType, in.Description, in.Instructions, in.DurationMinutes, utcTime(in.StartAt), utcTime(in.EndAt))
		} else {
			_, err = q.ExecContext(ctx, `UPDATE assessments SET subject_id=?,title=?,assessment_type=?,description=?,instructions=?,duration_minutes=?,start_at=?,end_at=? WHERE id=?`, in.SubjectID, in.Title, in.AssessmentType, in.Description, in.Instructions, in.DurationMinutes, utcTime(in.StartAt), utcTime(in.EndAt), id)
		}
		if err != nil {
			return err
		}
		if _, err = q.ExecContext(ctx, "DELETE FROM assessment_targets WHERE assessment_id=?", id); err != nil {
			return err
		}
		if _, err = q.ExecContext(ctx, "DELETE FROM assessment_questions WHERE assessment_id=?", id); err != nil {
			return err
		}
		for _, cid := range in.ClassIDs {
			if _, err = q.ExecContext(ctx, "INSERT INTO assessment_targets (assessment_id,class_id) VALUES (?,?)", id, cid); err != nil {
				return err
			}
		}
		for i, v := range in.Questions {
			qid, err := insertID(ctx, q, "INSERT INTO assessment_questions (assessment_id,question_type,question_text,points,order_no) VALUES (?,?,?,?,?)", id, v.Type, v.Text, v.Points, i+1)
			if err != nil {
				return err
			}
			for j, o := range v.Options {
				correct := o.Correct != nil && *o.Correct
				if _, err = q.ExecContext(ctx, "INSERT INTO question_options (question_id,option_text,is_correct,order_no) VALUES (?,?,?,?)", qid, o.Text, correct, j+1); err != nil {
					return err
				}
			}
		}
		return audit(ctx, q, a, "save_draft", "assessments", id)
	})
	return id, err
}
func utcTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}
func (s *Service) PublishAssessment(ctx context.Context, a Actor, id uint64) error {
	if a.Role != "teacher" {
		return forbidden
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		owner, status, err := lockAssessment(ctx, q, id)
		if err != nil {
			return err
		}
		if owner != a.ID {
			return forbidden
		}
		if status != "draft" {
			return conflict("Asesmen sudah diterbitkan.")
		}
		in, _, err := readAssessment(ctx, q, id)
		if err != nil {
			return err
		}
		if err = validateAssessment(in, true); err != nil {
			return err
		}
		var active bool
		if err = q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM subjects WHERE id=? AND is_active=TRUE)", in.SubjectID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return invalid("Mata pelajaran sudah tidak aktif.")
		}
		for _, cid := range in.ClassIDs {
			if err = requireTeaching(ctx, q, a, cid, in.SubjectID); err != nil {
				return err
			}
		}
		if _, err = q.ExecContext(ctx, "UPDATE assessments SET status='published',max_attempts=1,result_release_mode='manual' WHERE id=?", id); err != nil {
			return err
		}
		return audit(ctx, q, a, "publish", "assessments", id)
	})
}
func (h *Handler) AssessmentEditor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	in, err := h.service.AssessmentEditor(r.Context(), actor(r), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, in)
}
func (h *Handler) SaveAssessment(w http.ResponseWriter, r *http.Request) {
	var id uint64
	if r.PathValue("id") != "" {
		var ok bool
		id, ok = pathID(w, r)
		if !ok {
			return
		}
	}
	var in AssessmentDraft
	if !decodeLimit(w, r, &in, 1024*1024) {
		return
	}
	saved, err := h.service.SaveAssessment(r.Context(), actor(r), id, in)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"id": saved})
}
func (h *Handler) PublishAssessment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.service.PublishAssessment(r.Context(), actor(r), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "published"})
}
