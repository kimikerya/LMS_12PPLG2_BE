package classroom

import (
	"context"
	"lms-website-be/internal/database"
)

type accessError string

func (e accessError) Error() string { return string(e) }

func classAccess(ctx context.Context, q database.Querier, id, userID uint64, role string, manage bool) error {
	if role == "admin" {
		return nil
	}
	var allowed bool
	query := "SELECT EXISTS(SELECT 1 FROM class_teachers WHERE class_id=? AND teacher_user_id=? AND status='active')"
	if role == "student" && !manage {
		query = "SELECT EXISTS(SELECT 1 FROM class_members WHERE class_id=? AND student_user_id=? AND status='active')"
	} else if role != "teacher" {
		return accessError("Anda tidak memiliki akses ke kelas ini.")
	}
	if err := q.QueryRowContext(ctx, query, id, userID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return accessError("Anda tidak memiliki akses ke kelas ini.")
	}
	return nil
}
