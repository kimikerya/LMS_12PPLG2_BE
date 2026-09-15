package learning

import (
	"context"
	"database/sql"
	"errors"
	"lms-website-be/internal/database"
)

type Repository struct {
	q        database.Querier
	transact func(context.Context, func(database.Querier) error) error
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{q: db, transact: func(ctx context.Context, fn func(database.Querier) error) error { return database.WithTx(ctx, db, fn) }}
}

// Table/column names are fixed here; request values are always SQL parameters.
func contentQuery(kind string, actor Actor) (string, []any, error) {
	var projection string
	switch kind {
	case "materials":
		projection = "x.class_id, x.subject_id, x.title, x.status, x.published_at, NULL, x.description, x.url, x.file_path, NULL, x.material_type"
	case "assignments":
		projection = "x.class_id, x.subject_id, x.title, x.status, x.published_at, x.due_at, x.instructions, NULL, NULL, x.max_points, NULL"
	case "assessments":
		projection = "NULL, x.subject_id, x.title, x.status, NULL, NULL, x.description, NULL, NULL, NULL, x.assessment_type"
	default:
		return "", nil, notFound
	}
	extra := "NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL"
	switch kind {
	case "materials":
		extra = "x.meeting_no, NULL, NULL, NULL, NULL, NULL, NULL, NULL"
	case "assignments":
		extra = "NULL, NULL, NULL, NULL, NULL, x.close_at, x.allow_late, NULL"
	case "assessments":
		extra = "NULL, x.start_at, x.end_at, x.duration_minutes, x.instructions, NULL, NULL, (SELECT COUNT(*) FROM assessment_questions aq WHERE aq.assessment_id=x.id)"
	}
	query := "SELECT x.id, x.teacher_user_id, " + projection + ", (SELECT u.full_name FROM users u WHERE u.id=x.teacher_user_id), " + extra
	args := []any{}
	if kind == "assignments" && actor.Role == "student" {
		query += ", (SELECT CASE WHEN s.status='graded' AND s.result_released_at IS NULL THEN 'submitted' ELSE s.status END FROM assignment_submissions s WHERE s.assignment_id=x.id AND s.student_user_id=?), (SELECT s.submitted_at FROM assignment_submissions s WHERE s.assignment_id=x.id AND s.student_user_id=?)"
		args = append(args, actor.ID, actor.ID)
	} else {
		query += ", NULL, NULL"
	}
	query += " FROM " + kind + " x WHERE x.deleted_at IS NULL"
	switch {
	case monitoring(actor.Role):
	case actor.Role == "teacher":
		query += " AND x.teacher_user_id = ?"
		args = append(args, actor.ID)
	case actor.Role == "student":
		if kind == "assessments" {
			query += " AND (x.status='published' OR (x.status='closed' AND EXISTS(SELECT 1 FROM assessment_attempts ar WHERE ar.assessment_id=x.id AND ar.student_user_id=? AND ar.status IN ('submitted','graded'))))"
			args = append(args, actor.ID)
		} else if kind == "assignments" {
			query += " AND (x.status='published' OR (x.status='closed' AND EXISTS(SELECT 1 FROM assignment_submissions sr WHERE sr.assignment_id=x.id AND sr.student_user_id=?)))"
			args = append(args, actor.ID)
		} else {
			query += " AND x.status = 'published'"
		}
		if kind == "assessments" {
			query += ` AND EXISTS (SELECT 1 FROM assessment_targets atg JOIN class_members cm ON cm.class_id = atg.class_id JOIN classes c ON c.id = cm.class_id WHERE atg.assessment_id = x.id AND cm.student_user_id = ? AND cm.status = 'active' AND c.status = 'active')`
		} else {
			query += ` AND EXISTS (SELECT 1 FROM class_members cm JOIN classes c ON c.id = cm.class_id WHERE cm.class_id = x.class_id AND cm.student_user_id = ? AND cm.status = 'active' AND c.status = 'active')`
		}
		args = append(args, actor.ID)
	default:
		return "", nil, forbidden
	}
	return query, args, nil
}

func (r *Repository) List(ctx context.Context, actor Actor, kind string, f Filter, id uint64) ([]Content, error) {
	query, args, err := contentQuery(kind, actor)
	if err != nil {
		return nil, err
	}
	if id != 0 {
		query += " AND x.id = ?"
		args = append(args, id)
	}
	if f.SubjectID != 0 {
		query += " AND x.subject_id = ?"
		args = append(args, f.SubjectID)
	}
	if f.ClassID != 0 {
		if kind == "assessments" {
			query += " AND EXISTS (SELECT 1 FROM assessment_targets ft WHERE ft.assessment_id = x.id AND ft.class_id = ?)"
		} else {
			query += " AND x.class_id = ?"
		}
		args = append(args, f.ClassID)
	}
	query += " ORDER BY x.id DESC LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)
	rows, err := r.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Content{}
	for rows.Next() {
		var v Content
		if err := rows.Scan(&v.ID, &v.TeacherID, &v.ClassID, &v.SubjectID, &v.Title, &v.Status, &v.PublishedAt, &v.DueAt, &v.Description, &v.URL, &v.FilePath, &v.MaxPoints, &v.Type, &v.TeacherName, &v.MeetingNo, &v.StartAt, &v.EndAt, &v.DurationMinutes, &v.Instructions, &v.CloseAt, &v.AllowLate, &v.QuestionCount, &v.SubmissionStatus, &v.SubmittedAt); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

func lockClass(ctx context.Context, q database.Querier, classID uint64) error {
	var status string
	err := q.QueryRowContext(ctx, "SELECT status FROM classes WHERE id = ? FOR UPDATE", classID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound
	}
	if err != nil {
		return err
	}
	if status != "active" {
		return conflict("kelas sudah diarsipkan")
	}
	return nil
}
func requireTeaching(ctx context.Context, q database.Querier, actor Actor, classID uint64, subjectID *uint64) error {
	if actor.Role != "teacher" {
		return forbidden
	}
	if err := lockClass(ctx, q, classID); err != nil {
		return err
	}
	var allowed bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM class_teachers WHERE class_id = ? AND teacher_user_id = ? AND status = 'active' AND (? IS NULL OR subject_id = ?))`, classID, actor.ID, subjectID, subjectID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return forbidden
	}
	return nil
}
func requireMember(ctx context.Context, q database.Querier, studentID, classID uint64) error {
	var allowed bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM class_members cm JOIN classes c ON c.id = cm.class_id WHERE cm.class_id = ? AND cm.student_user_id = ? AND cm.status = 'active' AND c.status = 'active')`, classID, studentID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return forbidden
	}
	return nil
}
func insertID(ctx context.Context, q database.Querier, query string, args ...any) (uint64, error) {
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return uint64(id), err
}
func audit(ctx context.Context, q database.Querier, actor Actor, action, entity string, id uint64) error {
	_, err := q.ExecContext(ctx, `INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id) VALUES (?, ?, ?, ?)`, actor.ID, action, entity, id)
	return err
}
func (r *Repository) create(ctx context.Context, actor Actor, classID uint64, subjectID *uint64, kind, query string, args ...any) (uint64, error) {
	var id uint64
	err := r.transact(ctx, func(q database.Querier) error {
		if classID != 0 {
			if err := requireTeaching(ctx, q, actor, classID, subjectID); err != nil {
				return err
			}
		}
		if subjectID != nil {
			var exists bool
			if err := q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM subjects WHERE id = ? AND is_active = TRUE)", *subjectID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return invalid("mata pelajaran tidak aktif atau tidak ditemukan")
			}
		}
		var err error
		id, err = insertID(ctx, q, query, args...)
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, "create", kind, id)
	})
	return id, err
}

func (r *Repository) Publish(ctx context.Context, actor Actor, kind string, id uint64) error {
	if kind != "assignments" && kind != "materials" {
		return invalid("publish jenis ini belum tersedia")
	}
	return r.transact(ctx, func(q database.Querier) error {
		var owner, classID uint64
		var subjectID *uint64
		var status string
		err := q.QueryRowContext(ctx, "SELECT teacher_user_id, class_id, subject_id, status FROM "+kind+" WHERE id = ? AND deleted_at IS NULL FOR UPDATE", id).Scan(&owner, &classID, &subjectID, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound
		}
		if err != nil {
			return err
		}
		if owner != actor.ID || actor.Role != "teacher" {
			return forbidden
		}
		if err := requireTeaching(ctx, q, actor, classID, subjectID); err != nil {
			return err
		}
		if status == "published" {
			return nil
		}
		if status != "draft" {
			return conflict("hanya draft yang dapat diterbitkan")
		}
		_, err = q.ExecContext(ctx, "UPDATE "+kind+" SET status = 'published', published_at = CURRENT_TIMESTAMP WHERE id = ?", id)
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, "publish", kind, id)
	})
}
