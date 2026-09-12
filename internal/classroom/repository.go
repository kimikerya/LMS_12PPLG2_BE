package classroom

import (
	"context"
	"database/sql"
	"fmt"
	"lms-website-be/internal/database"
)

type Repository struct {
	db       database.Querier
	transact func(context.Context, func(database.Querier) error) error
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, transact: func(ctx context.Context, fn func(database.Querier) error) error { return database.WithTx(ctx, db, fn) }}
}

func (r *Repository) ListForUser(ctx context.Context, userID uint64, role string) ([]Class, error) {
	query := `SELECT DISTINCT c.id, c.academic_year_id, c.education_level_id, c.major_id, c.grade_level, c.title, c.description, c.room, c.status, c.created_by FROM classes c`
	args := []any{}
	switch role {
	case "student":
		query += " JOIN class_members cm ON cm.class_id = c.id AND cm.student_user_id = ? AND cm.status = 'active'"
		args = append(args, userID)
	case "teacher":
		query += " JOIN class_teachers ct ON ct.class_id = c.id AND ct.teacher_user_id = ? AND ct.status = 'active'"
		args = append(args, userID)
	case "admin", "curriculum", "principal":
	default:
		return nil, fmt.Errorf("role tidak diizinkan")
	}
	query += " WHERE c.status = 'active' ORDER BY c.grade_level, c.title"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mengambil daftar kelas: %w", err)
	}
	defer rows.Close()
	items := make([]Class, 0)
	for rows.Next() {
		var item Class
		if err := rows.Scan(&item.ID, &item.AcademicYearID, &item.EducationLevelID, &item.MajorID, &item.GradeLevel, &item.Title, &item.Description, &item.Room, &item.Status, &item.CreatedBy); err != nil {
			return nil, fmt.Errorf("membaca kelas: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Create(ctx context.Context, input CreateInput, createdBy uint64, role string) (Class, error) {
	var item Class
	if role != "admin" && role != "teacher" {
		return item, accessError("Hanya admin dan guru dapat membuat kelas.")
	}
	err := r.transact(ctx, func(q database.Querier) error {
		if err := roleCheck(ctx, q, createdBy, role, true); err != nil {
			return err
		}
		if role == "teacher" {
			if input.SubjectID == nil {
				return membershipError("Pilih mata pelajaran Anda.")
			}
			var exists bool
			if err := q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM subjects WHERE id=? AND is_active=TRUE)", *input.SubjectID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return membershipError("Mata pelajaran tidak tersedia.")
			}
		}
		result, err := q.ExecContext(ctx, "INSERT INTO classes (academic_year_id,education_level_id,major_id,grade_level,title,description,room,created_by) VALUES (?,?,?,?,?,?,?,?)", input.AcademicYearID, input.EducationLevelID, input.MajorID, input.GradeLevel, input.Title, input.Description, input.Room, createdBy)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if role == "teacher" {
			if _, err = q.ExecContext(ctx, "INSERT INTO class_teachers(class_id,teacher_user_id,subject_id,role) VALUES (?,?,?,'subject_teacher')", id, createdBy, input.SubjectID); err != nil {
				return err
			}
		}
		return q.QueryRowContext(ctx, "SELECT id,academic_year_id,education_level_id,major_id,grade_level,title,description,room,status,created_by FROM classes WHERE id=?", id).Scan(&item.ID, &item.AcademicYearID, &item.EducationLevelID, &item.MajorID, &item.GradeLevel, &item.Title, &item.Description, &item.Room, &item.Status, &item.CreatedBy)
	})
	return item, err
}
