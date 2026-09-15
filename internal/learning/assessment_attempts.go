package learning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"lms-website-be/internal/database"
	"math"
	"net/http"
	"time"
)

type ExamAnswer struct {
	QuestionID uint64   `json:"question_id"`
	Text       string   `json:"text"`
	OptionIDs  []uint64 `json:"option_ids"`
	Points     *float64 `json:"points,omitempty"`
	Feedback   string   `json:"feedback,omitempty"`
}
type ExamPaper struct {
	ID           uint64               `json:"id"`
	Status       string               `json:"status"`
	Title        string               `json:"title"`
	Instructions *string              `json:"instructions"`
	ExpiresAt    time.Time            `json:"expires_at"`
	ServerNow    time.Time            `json:"server_now"`
	Questions    []AssessmentQuestion `json:"questions"`
	Answers      []ExamAnswer         `json:"answers"`
}
type AttemptSummary struct {
	ID          uint64       `json:"id"`
	StudentName string       `json:"student_name"`
	StudentID   string       `json:"student_id"`
	ClassName   string       `json:"class_name"`
	Status      string       `json:"status"`
	Score       *float64     `json:"score"`
	ReleasedAt  *time.Time   `json:"released_at"`
	Answers     []ExamAnswer `json:"answers"`
}

func examDeadline(start time.Time, in AssessmentDraft) time.Time {
	duration := uint16(60)
	if in.DurationMinutes != nil {
		duration = *in.DurationMinutes
	}
	end := start.Add(time.Duration(duration) * time.Minute)
	if in.EndAt != nil && in.EndAt.Before(end) {
		end = *in.EndAt
	}
	return end
}
func examAnswers(ctx context.Context, q database.Querier, id uint64) ([]ExamAnswer, error) {
	out := []ExamAnswer{}
	rows, err := q.QueryContext(ctx, "SELECT question_id,COALESCE(essay_text,''),awarded_points,COALESCE(teacher_feedback,'') FROM assessment_answers WHERE attempt_id=? ORDER BY question_id", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		v := ExamAnswer{OptionIDs: []uint64{}}
		if err = rows.Scan(&v.QuestionID, &v.Text, &v.Points, &v.Feedback); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.QueryContext(ctx, `SELECT aa.question_id,ao.option_id FROM assessment_answers aa JOIN assessment_answer_options ao ON ao.answer_id=aa.id WHERE aa.attempt_id=?`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var qid, oid uint64
		if err = rows.Scan(&qid, &oid); err != nil {
			rows.Close()
			return nil, err
		}
		for i := range out {
			if out[i].QuestionID == qid {
				out[i].OptionIDs = append(out[i].OptionIDs, oid)
			}
		}
	}
	err = rows.Err()
	rows.Close()
	return out, err
}
func saveExamAnswers(ctx context.Context, q database.Querier, id uint64, in AssessmentDraft, answers []ExamAnswer) error {
	if len(answers) > len(in.Questions) {
		return invalid("Jumlah jawaban tidak valid.")
	}
	known := map[uint64]AssessmentQuestion{}
	for _, v := range in.Questions {
		known[v.ID] = v
	}
	seen := map[uint64]bool{}
	for _, a := range answers {
		question, ok := known[a.QuestionID]
		if !ok || seen[a.QuestionID] {
			return invalid("Soal tidak sesuai dengan asesmen.")
		}
		seen[a.QuestionID] = true
		if len(a.Text) > 10000 || a.Points != nil || a.Feedback != "" {
			return invalid("Isi jawaban tidak valid.")
		}
		allowed := map[uint64]bool{}
		for _, o := range question.Options {
			allowed[o.ID] = true
		}
		selected := map[uint64]bool{}
		for _, oid := range a.OptionIDs {
			if !allowed[oid] || selected[oid] {
				return invalid("Pilihan jawaban tidak sesuai dengan soal.")
			}
			selected[oid] = true
		}
		if question.Type == "essay" && len(a.OptionIDs) > 0 || question.Type != "essay" && a.Text != "" || question.Type == "single_choice" && len(a.OptionIDs) > 1 {
			return invalid("Jawaban tidak sesuai dengan jenis soal.")
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO assessment_answers (attempt_id,question_id,essay_text) VALUES (?,?,?) ON DUPLICATE KEY UPDATE essay_text=VALUES(essay_text)`, id, a.QuestionID, a.Text); err != nil {
			return err
		}
		var aid uint64
		if err := q.QueryRowContext(ctx, "SELECT id FROM assessment_answers WHERE attempt_id=? AND question_id=?", id, a.QuestionID).Scan(&aid); err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx, "DELETE FROM assessment_answer_options WHERE answer_id=?", aid); err != nil {
			return err
		}
		for _, oid := range a.OptionIDs {
			if _, err := q.ExecContext(ctx, "INSERT INTO assessment_answer_options (answer_id,option_id) VALUES (?,?)", aid, oid); err != nil {
				return err
			}
		}
	}
	return nil
}
func finalizeExam(ctx context.Context, q database.Querier, id uint64, in AssessmentDraft) error {
	answers, err := examAnswers(ctx, q, id)
	if err != nil {
		return err
	}
	selected := map[uint64]map[uint64]bool{}
	for _, a := range answers {
		selected[a.QuestionID] = map[uint64]bool{}
		for _, oid := range a.OptionIDs {
			selected[a.QuestionID][oid] = true
		}
	}
	total := 0.0
	hasEssay := false
	for _, v := range in.Questions {
		var points *float64
		if v.Type == "essay" {
			hasEssay = true
		} else {
			score := 0.0
			correct := true
			for _, o := range v.Options {
				if selected[v.ID][o.ID] != (o.Correct != nil && *o.Correct) {
					correct = false
				}
			}
			if correct {
				score = v.Points
			}
			points = &score
			total += score
		}
		if _, err = q.ExecContext(ctx, `INSERT INTO assessment_answers (attempt_id,question_id,awarded_points) VALUES (?,?,?) ON DUPLICATE KEY UPDATE awarded_points=VALUES(awarded_points)`, id, v.ID, points); err != nil {
			return err
		}
	}
	status := "graded"
	var final *float64
	total = math.Round(total*100) / 100
	if hasEssay {
		status = "submitted"
	} else {
		final = &total
	}
	_, err = q.ExecContext(ctx, `UPDATE assessment_attempts SET status=?,submitted_at=UTC_TIMESTAMP(),auto_score=?,final_score=?,graded_at=IF(?='graded',UTC_TIMESTAMP(),NULL),result_released_at=IF(?='graded',UTC_TIMESTAMP(),NULL) WHERE id=?`, status, total, final, status, status, id)
	return err
}
func (s *Service) StartExam(ctx context.Context, a Actor, id uint64) (ExamPaper, error) {
	paper := ExamPaper{Questions: []AssessmentQuestion{}, Answers: []ExamAnswer{}}
	if a.Role != "student" {
		return paper, forbidden
	}
	err := s.repository.transact(ctx, func(q database.Querier) error {
		_, status, err := lockAssessment(ctx, q, id)
		if err != nil {
			return err
		}
		in, _, err := readAssessment(ctx, q, id)
		if err != nil {
			return err
		}
		var cid uint64
		err = q.QueryRowContext(ctx, `SELECT cm.class_id FROM class_members cm JOIN classes c ON c.id=cm.class_id JOIN assessment_targets t ON t.class_id=cm.class_id WHERE t.assessment_id=? AND cm.student_user_id=? AND cm.status='active' AND c.status='active' ORDER BY cm.class_id LIMIT 1`, id, a.ID).Scan(&cid)
		if errors.Is(err, sql.ErrNoRows) {
			return forbidden
		}
		if err != nil {
			return err
		}
		var started *time.Time
		err = q.QueryRowContext(ctx, "SELECT id,status,started_at FROM assessment_attempts WHERE assessment_id=? AND student_user_id=? ORDER BY id DESC LIMIT 1 FOR UPDATE", id, a.ID).Scan(&paper.ID, &paper.Status, &started)
		now := time.Now().UTC()
		if errors.Is(err, sql.ErrNoRows) || err == nil && paper.Status == "not_started" {
			if status != "published" || in.StartAt == nil || now.Before(*in.StartAt) || in.EndAt == nil || !now.Before(*in.EndAt) {
				return conflict("Asesmen belum dimulai atau sudah ditutup.")
			}
			if in.DurationMinutes == nil || len(in.Questions) == 0 {
				return conflict("Asesmen belum siap dikerjakan.")
			}
			if paper.ID == 0 {
				paper.ID, err = insertID(ctx, q, "INSERT INTO assessment_attempts (assessment_id,student_user_id,class_id,started_at,status) VALUES (?,?,?,?,'in_progress')", id, a.ID, cid, now)
			} else {
				_, err = q.ExecContext(ctx, "UPDATE assessment_attempts SET started_at=?,status='in_progress',class_id=? WHERE id=?", now, cid, paper.ID)
			}
			if err != nil {
				return err
			}
			started = &now
			paper.Status = "in_progress"
		} else if err != nil {
			return err
		}
		if status == "draft" {
			return forbidden
		}
		paper.Title = in.Title
		paper.Instructions = in.Instructions
		paper.ExpiresAt = now
		if started != nil {
			paper.ExpiresAt = examDeadline(*started, in)
		}
		paper.ServerNow = now
		if paper.Status == "in_progress" && (!now.Before(paper.ExpiresAt) || status == "closed") {
			if err = finalizeExam(ctx, q, paper.ID, in); err != nil {
				return err
			}
			paper.Status = "submitted"
		}
		if paper.Status == "in_progress" {
			paper.Questions = in.Questions
			for i := range paper.Questions {
				for j := range paper.Questions[i].Options {
					paper.Questions[i].Options[j].Correct = nil
				}
			}
			paper.Answers, err = examAnswers(ctx, q, paper.ID)
			if err != nil {
				return err
			}
			for i := range paper.Answers {
				paper.Answers[i].Points = nil
				paper.Answers[i].Feedback = ""
			}
		}
		return nil
	})
	return paper, err
}
func (s *Service) SaveExam(ctx context.Context, a Actor, id uint64, answers []ExamAnswer, submit bool) (string, error) {
	if a.Role != "student" {
		return "", forbidden
	}
	result := "in_progress"
	err := s.repository.transact(ctx, func(q database.Querier) error {
		var assessmentID uint64
		if err := q.QueryRowContext(ctx, "SELECT assessment_id FROM assessment_attempts WHERE id=? AND student_user_id=?", id, a.ID).Scan(&assessmentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return notFound
			}
			return err
		}
		_, assessmentStatus, err := lockAssessment(ctx, q, assessmentID)
		if err != nil {
			return err
		}
		var status string
		var started *time.Time
		if err = q.QueryRowContext(ctx, "SELECT status,started_at FROM assessment_attempts WHERE id=? FOR UPDATE", id).Scan(&status, &started); err != nil {
			return err
		}
		if status != "in_progress" {
			result = status
			return nil
		}
		var permitted int
		if err = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM assessment_attempts a JOIN class_members cm ON cm.class_id=a.class_id AND cm.student_user_id=a.student_user_id JOIN classes c ON c.id=cm.class_id JOIN assessment_targets t ON t.class_id=c.id AND t.assessment_id=a.assessment_id WHERE a.id=? AND cm.status='active' AND c.status='active'`, id).Scan(&permitted); err != nil {
			return err
		}
		if permitted == 0 || assessmentStatus == "draft" {
			return forbidden
		}
		in, _, err := readAssessment(ctx, q, assessmentID)
		if err != nil {
			return err
		}
		expired := started == nil || !time.Now().Before(examDeadline(*started, in)) || assessmentStatus == "closed"
		// Once time is up, only answers already saved on the server count.
		if !expired {
			if err = saveExamAnswers(ctx, q, id, in, answers); err != nil {
				return err
			}
		}
		if expired || submit {
			if err = finalizeExam(ctx, q, id, in); err != nil {
				return err
			}
			result = "submitted"
			return audit(ctx, q, a, "submit", "assessment_attempts", id)
		}
		return nil
	})
	return result, err
}
func (s *Service) ExamAttempts(ctx context.Context, a Actor, id uint64) ([]AttemptSummary, error) {
	if a.Role != "teacher" {
		return nil, forbidden
	}
	out := []AttemptSummary{}
	err := s.repository.transact(ctx, func(q database.Querier) error {
		owner, _, err := lockAssessment(ctx, q, id)
		if err != nil {
			return err
		}
		if owner != a.ID {
			return forbidden
		}
		in, _, err := readAssessment(ctx, q, id)
		if err != nil {
			return err
		}
		rows, err := q.QueryContext(ctx, `SELECT a.id,u.full_name,u.login_id,c.title,a.status,a.final_score,a.result_released_at,a.started_at FROM assessment_attempts a JOIN users u ON u.id=a.student_user_id JOIN classes c ON c.id=a.class_id WHERE a.assessment_id=? ORDER BY u.full_name,a.id`, id)
		if err != nil {
			return err
		}
		starts := []*time.Time{}
		for rows.Next() {
			v := AttemptSummary{Answers: []ExamAnswer{}}
			var started *time.Time
			if err = rows.Scan(&v.ID, &v.StudentName, &v.StudentID, &v.ClassName, &v.Status, &v.Score, &v.ReleasedAt, &started); err != nil {
				rows.Close()
				return err
			}
			out = append(out, v)
			starts = append(starts, started)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for i := range out {
			if out[i].Status == "in_progress" && starts[i] != nil && !time.Now().Before(examDeadline(*starts[i], in)) {
				if err = finalizeExam(ctx, q, out[i].ID, in); err != nil {
					return err
				}
				if err = q.QueryRowContext(ctx, "SELECT status,final_score FROM assessment_attempts WHERE id=?", out[i].ID).Scan(&out[i].Status, &out[i].Score); err != nil {
					return err
				}
			}
			if out[i].Status != "in_progress" && out[i].Status != "not_started" {
				out[i].Answers, err = examAnswers(ctx, q, out[i].ID)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	return out, err
}
func (s *Service) GradeExam(ctx context.Context, a Actor, id uint64, answers []ExamAnswer, release bool) error {
	if a.Role != "teacher" {
		return forbidden
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		var assessmentID uint64
		if err := q.QueryRowContext(ctx, "SELECT assessment_id FROM assessment_attempts WHERE id=?", id).Scan(&assessmentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return notFound
			}
			return err
		}
		owner, _, err := lockAssessment(ctx, q, assessmentID)
		if err != nil {
			return err
		}
		if owner != a.ID {
			return forbidden
		}
		var status string
		var released *time.Time
		if err = q.QueryRowContext(ctx, "SELECT status,result_released_at FROM assessment_attempts WHERE id=? FOR UPDATE", id).Scan(&status, &released); err != nil {
			return err
		}
		if release {
			if status != "graded" {
				return conflict("Selesaikan penilaian semua soal sebelum merilis nilai.")
			}
			if _, err = q.ExecContext(ctx, "UPDATE assessment_attempts SET result_released_at=UTC_TIMESTAMP() WHERE id=?", id); err != nil {
				return err
			}
			return audit(ctx, q, a, "release", "assessment_attempts", id)
		}
		if released != nil {
			return conflict("Nilai sudah dirilis dan tidak dapat diubah.")
		}
		if status != "submitted" && status != "graded" {
			return conflict("Siswa belum mengumpulkan jawaban.")
		}
		in, _, err := readAssessment(ctx, q, assessmentID)
		if err != nil {
			return err
		}
		essays := map[uint64]AssessmentQuestion{}
		for _, v := range in.Questions {
			if v.Type == "essay" {
				essays[v.ID] = v
			}
		}
		if len(answers) != len(essays) {
			return invalid("Nilai semua jawaban esai terlebih dahulu.")
		}
		seen := map[uint64]bool{}
		manual := 0.0
		for _, v := range answers {
			question, ok := essays[v.QuestionID]
			if !ok || seen[v.QuestionID] || v.Points == nil || !validPoints(*v.Points) || *v.Points > question.Points || len(v.Feedback) > 10000 {
				return invalid(fmt.Sprintf("Periksa nilai dan umpan balik soal %d.", v.QuestionID))
			}
			seen[v.QuestionID] = true
			manual += *v.Points
			if _, err = q.ExecContext(ctx, "UPDATE assessment_answers SET awarded_points=?,teacher_feedback=? WHERE attempt_id=? AND question_id=?", v.Points, v.Feedback, id, v.QuestionID); err != nil {
				return err
			}
		}
		manual = math.Round(manual*100) / 100
		if _, err = q.ExecContext(ctx, "UPDATE assessment_attempts SET manual_score=?,final_score=COALESCE(auto_score,0)+?,status='graded',graded_by=?,graded_at=UTC_TIMESTAMP() WHERE id=?", manual, manual, a.ID, id); err != nil {
			return err
		}
		return audit(ctx, q, a, "grade", "assessment_attempts", id)
	})
}
func (h *Handler) StartExam(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	paper, err := h.service.StartExam(r.Context(), actor(r), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, paper)
}
func (h *Handler) SaveExam(submit bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var in struct {
			Answers []ExamAnswer `json:"answers"`
		}
		if !decodeLimit(w, r, &in, 1024*1024) {
			return
		}
		status, err := h.service.SaveExam(r.Context(), actor(r), id, in.Answers, submit)
		if err != nil {
			respondError(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"status": status})
	}
}
func (h *Handler) ExamAttempts(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	items, err := h.service.ExamAttempts(r.Context(), actor(r), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}
func (h *Handler) GradeExam(release bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var in struct {
			Answers []ExamAnswer `json:"answers"`
		}
		if !release && !decodeLimit(w, r, &in, 1024*1024) {
			return
		}
		if err := h.service.GradeExam(r.Context(), actor(r), id, in.Answers, release); err != nil {
			respondError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}
