package learning

import (
	"context"
	"net/http"
	"time"
)

type AssessmentReview struct {
	ID          uint64         `json:"id"`
	SubmittedAt *time.Time     `json:"submitted_at"`
	ReleasedAt  *time.Time     `json:"result_released_at"`
	Score       *float64       `json:"score"`
	MaxPoints   *float64       `json:"max_points"`
	Answers     []AnswerReview `json:"answers"`
}
type AnswerReview struct {
	ID        uint64   `json:"id"`
	Order     int      `json:"order"`
	Question  string   `json:"question"`
	Answer    *string  `json:"answer"`
	Points    *float64 `json:"points"`
	MaxPoints float64  `json:"max_points"`
	Feedback  *string  `json:"feedback"`
	Options   []string `json:"options"`
}

func (s *Service) AssessmentResults(ctx context.Context, a Actor, id uint64) ([]AssessmentReview, error) {
	if a.Role != "student" {
		return nil, forbidden
	}
	// Reuse class targeting and membership checks; never accept a student ID from the request.
	visible, err := s.List(ctx, a, "assessments", Filter{Limit: 1}, id)
	if err != nil {
		return nil, err
	}
	if len(visible) == 0 {
		return nil, notFound
	}
	// Settle expired work even if the browser was closed before its timer finished.
	var pending bool
	if err = s.repository.q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM assessment_attempts WHERE assessment_id=? AND student_user_id=? AND status='in_progress')", id, a.ID).Scan(&pending); err != nil {
		return nil, err
	}
	if pending {
		if _, err = s.StartExam(ctx, a, id); err != nil {
			return nil, err
		}
	}
	rows, err := s.repository.q.QueryContext(ctx, `SELECT id,submitted_at,result_released_at,final_score
        FROM assessment_attempts WHERE assessment_id=? AND student_user_id=? AND status IN ('submitted','graded')
        ORDER BY id DESC`, id, a.ID)
	if err != nil {
		return nil, err
	}
	results := []AssessmentReview{}
	for rows.Next() {
		v := AssessmentReview{Answers: []AnswerReview{}}
		if err = rows.Scan(&v.ID, &v.SubmittedAt, &v.ReleasedAt, &v.Score); err != nil {
			rows.Close()
			return nil, err
		}
		if v.ReleasedAt == nil || v.ReleasedAt.After(time.Now()) {
			v.ReleasedAt = nil
			v.Score = nil
		}
		results = append(results, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range results {
		v := &results[i]
		if v.ReleasedAt == nil {
			continue
		}
		if err = s.repository.q.QueryRowContext(ctx, "SELECT COALESCE(SUM(points),0) FROM assessment_questions WHERE assessment_id=?", id).Scan(&v.MaxPoints); err != nil {
			return nil, err
		}
		answers, err := s.repository.q.QueryContext(ctx, `SELECT q.id,q.order_no,q.question_text,aa.essay_text,aa.awarded_points,q.points,aa.teacher_feedback
            FROM assessment_questions q LEFT JOIN assessment_answers aa ON aa.question_id=q.id AND aa.attempt_id=?
            WHERE q.assessment_id=? ORDER BY q.order_no`, v.ID, id)
		if err != nil {
			return nil, err
		}
		for answers.Next() {
			r := AnswerReview{Options: []string{}}
			if err = answers.Scan(&r.ID, &r.Order, &r.Question, &r.Answer, &r.Points, &r.MaxPoints, &r.Feedback); err != nil {
				answers.Close()
				return nil, err
			}
			v.Answers = append(v.Answers, r)
		}
		err = answers.Err()
		answers.Close()
		if err != nil {
			return nil, err
		}
		// Return only the student's selected options, never correctness flags or answer keys.
		options, err := s.repository.q.QueryContext(ctx, `SELECT aa.question_id,qo.option_text FROM assessment_answers aa
            JOIN assessment_answer_options ao ON ao.answer_id=aa.id
            JOIN question_options qo ON qo.id=ao.option_id AND qo.question_id=aa.question_id
            WHERE aa.attempt_id=? ORDER BY qo.order_no`, v.ID)
		if err != nil {
			return nil, err
		}
		for options.Next() {
			var questionID uint64
			var text string
			if err = options.Scan(&questionID, &text); err != nil {
				options.Close()
				return nil, err
			}
			for j := range v.Answers {
				if v.Answers[j].ID == questionID {
					v.Answers[j].Options = append(v.Answers[j].Options, text)
				}
			}
		}
		err = options.Err()
		options.Close()
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (h *Handler) AssessmentResults(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	items, err := h.service.AssessmentResults(r.Context(), actor(r), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}
