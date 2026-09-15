package learning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"lms-website-be/internal/database"
	"time"
)

type lockedAssignment struct {
	Owner, ClassID uint64
	SubjectID      *uint64
	Status         string
	Due            time.Time
	CloseAt        *time.Time
	AllowLate      bool
	MaxPoints      float64
	Now            time.Time
}

func assignmentForUpdate(ctx context.Context, q database.Querier, id uint64) (lockedAssignment, error) {
	var v lockedAssignment
	err := q.QueryRowContext(ctx, `SELECT teacher_user_id, class_id, subject_id, status, due_at, close_at, allow_late, COALESCE(max_points, 100), UTC_TIMESTAMP() FROM assignments WHERE id = ? AND deleted_at IS NULL FOR UPDATE`, id).Scan(&v.Owner, &v.ClassID, &v.SubjectID, &v.Status, &v.Due, &v.CloseAt, &v.AllowLate, &v.MaxPoints, &v.Now)
	if errors.Is(err, sql.ErrNoRows) {
		return v, notFound
	}
	return v, err
}
func (r *Repository) Submit(ctx context.Context, a Actor, assignmentID uint64, in SubmitInput) (uint64, error) {
	var id uint64
	err := r.transact(ctx, func(q database.Querier) error {
		v, err := assignmentForUpdate(ctx, q, assignmentID)
		if err != nil {
			return err
		}
		if err := lockClass(ctx, q, v.ClassID); err != nil {
			return err
		}
		if err := requireMember(ctx, q, a.ID, v.ClassID); err != nil {
			return err
		}
		// Check time after acquiring locks, so waiting requests cannot bypass the cutoff.
		if err := q.QueryRowContext(ctx, "SELECT UTC_TIMESTAMP()").Scan(&v.Now); err != nil {
			return err
		}
		status, err := submissionStatus(v.Status, v.Due, v.CloseAt, v.AllowLate, v.Now)
		if err != nil {
			return err
		}
		var exists bool
		if err := q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM assignment_submissions WHERE assignment_id = ? AND student_user_id = ?)", assignmentID, a.ID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return conflict("tugas sudah dikumpulkan; pengiriman ulang belum tersedia")
		}
		id, err = insertID(ctx, q, `INSERT INTO assignment_submissions (assignment_id, student_user_id, submission_type, text_answer, link_url, submitted_at, status,submitted_late) VALUES (?, ?, ?, ?, ?, UTC_TIMESTAMP(), ?,?)`, assignmentID, a.ID, in.SubmissionType, in.TextAnswer, in.LinkURL, status, status == "late")
		if err != nil {
			return err
		}
		for _, file := range in.Files {
			if _, err = q.ExecContext(ctx, "INSERT INTO submission_files (submission_id,file_name,file_url) VALUES (?,?,?)", id, file.Name, file.URL); err != nil {
				return err
			}
		}
		return audit(ctx, q, a, "submit", "assignment_submissions", id)
	})
	return id, err
}
func (r *Repository) Submissions(ctx context.Context, a Actor, assignmentID uint64) ([]Submission, error) {
	var owner, classID uint64
	err := r.q.QueryRowContext(ctx, "SELECT teacher_user_id, class_id FROM assignments WHERE id = ? AND deleted_at IS NULL", assignmentID).Scan(&owner, &classID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notFound
	}
	if err != nil {
		return nil, err
	}
	query := `SELECT id, assignment_id, student_user_id, submission_type, text_answer, link_url, submitted_at, status, score, teacher_feedback, graded_at, result_released_at,COALESCE(submitted_late,submitted_at>(SELECT due_at FROM assignments x WHERE x.id=assignment_submissions.assignment_id)) FROM assignment_submissions WHERE assignment_id = ?`
	args := []any{assignmentID}
	switch {
	case a.Role == "student":
		if err := requireMember(ctx, r.q, a.ID, classID); err != nil {
			return nil, err
		}
		query += " AND student_user_id = ?"
		args = append(args, a.ID)
	case a.Role == "teacher" && a.ID == owner:
	case monitoring(a.Role):
	default:
		return nil, forbidden
	}
	// Return the complete roster for this assignment so monitoring does not
	// mistake submissions beyond the first 100 for missing work.
	query += " ORDER BY id DESC"
	rows, err := r.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Submission{}
	for rows.Next() {
		var v Submission
		if err := rows.Scan(&v.ID, &v.AssignmentID, &v.StudentID, &v.Type, &v.TextAnswer, &v.LinkURL, &v.SubmittedAt, &v.Status, &v.Score, &v.Feedback, &v.GradedAt, &v.ReleasedAt, &v.IsLate); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (r *Repository) Grade(ctx context.Context, a Actor, id uint64, in GradeInput, release bool) error {
	return r.transact(ctx, func(q database.Querier) error {
		var assignmentID uint64
		err := q.QueryRowContext(ctx, "SELECT assignment_id FROM assignment_submissions WHERE id = ?", id).Scan(&assignmentID)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound
		}
		if err != nil {
			return err
		}
		v, err := assignmentForUpdate(ctx, q, assignmentID)
		if err != nil {
			return err
		}
		if v.Owner != a.ID || a.Role != "teacher" {
			return forbidden
		}
		if err := requireTeaching(ctx, q, a, v.ClassID, v.SubjectID); err != nil {
			return err
		}
		var status string
		var releasedAt *time.Time
		var studentID uint64
		err = q.QueryRowContext(ctx, "SELECT status, result_released_at, student_user_id FROM assignment_submissions WHERE id = ? FOR UPDATE", id).Scan(&status, &releasedAt, &studentID)
		if err != nil {
			return err
		}
		if release {
			if status != "graded" {
				return conflict("jawaban belum dinilai")
			}
			if releasedAt != nil {
				return nil
			}
			if _, err := q.ExecContext(ctx, "UPDATE assignment_submissions SET result_released_at = UTC_TIMESTAMP() WHERE id = ?", id); err != nil {
				return err
			}
			_, err = q.ExecContext(ctx, `INSERT INTO notifications (user_id, type, title, message, link_url) VALUES (?, 'assignment_result', 'Nilai tugas tersedia', 'Guru telah merilis hasil tugas Anda.', ?)`, studentID, fmt.Sprintf("/assignments/%d", assignmentID))
			if err != nil {
				return err
			}
			return audit(ctx, q, a, "release", "assignment_submissions", id)
		}
		if releasedAt != nil {
			return conflict("nilai sudah dirilis; koreksi nilai memerlukan alur tersendiri")
		}
		if status != "submitted" && status != "late" && status != "graded" {
			return conflict("jawaban belum dikumpulkan")
		}
		if *in.Score > v.MaxPoints {
			return invalid("score melebihi max_points tugas")
		}
		_, err = q.ExecContext(ctx, `UPDATE assignment_submissions SET score = ?, teacher_feedback = ?, graded_by = ?, graded_at = UTC_TIMESTAMP(), status = 'graded' WHERE id = ?`, *in.Score, in.Feedback, a.ID, id)
		if err != nil {
			return err
		}
		return audit(ctx, q, a, "grade", "assignment_submissions", id)
	})
}
