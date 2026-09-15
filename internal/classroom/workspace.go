package classroom

import (
	"context"
	"database/sql"
	"net/http"

	"lms-website-be/internal/middleware"
)

// Apply the same scope to every query, including the roster and announcements.
// Client-provided class IDs must never widen access.
func workspaceScope(userID uint64, role string) (string, []any, error) {
	scope := "c.status='active'"
	args := []any{}
	switch role {
	case "admin", "curriculum", "principal":
	case "student":
		scope += " AND EXISTS(SELECT 1 FROM class_members access WHERE access.class_id=c.id AND access.student_user_id=? AND access.status='active')"
		args = append(args, userID)
	case "teacher":
		scope += " AND EXISTS(SELECT 1 FROM class_teachers access WHERE access.class_id=c.id AND access.teacher_user_id=? AND access.status='active')"
		args = append(args, userID)
	default:
		return "", nil, accessError("Anda tidak memiliki akses ke kelas ini.")
	}
	return scope, args, nil
}

// Four queries per workspace instead of four queries for every class.
// Students receive counts, never the other students' identity records.
func (s *Service) workspace(ctx context.Context, userID uint64, role string, id uint64, summary bool) ([]ClassDetail, error) {
	scope, args, err := workspaceScope(userID, role)
	if err != nil {
		return nil, err
	}
	if id != 0 {
		scope += " AND c.id=?"
		args = append(args, id)
	}
	q := s.repository.db
	rows, err := q.QueryContext(ctx, `SELECT c.id,c.academic_year_id,c.education_level_id,c.major_id,c.grade_level,c.title,c.description,c.room,c.status,c.created_by,y.name,l.name,m.name,
		(SELECT COUNT(*) FROM class_members cm JOIN users u ON u.id=cm.student_user_id WHERE cm.class_id=c.id AND cm.status='active' AND u.deleted_at IS NULL)
		FROM classes c JOIN academic_years y ON y.id=c.academic_year_id JOIN education_levels l ON l.id=c.education_level_id LEFT JOIN majors m ON m.id=c.major_id WHERE `+scope+` ORDER BY c.grade_level,c.title,c.id`, args...)
	if err != nil {
		return nil, err
	}
	items := []ClassDetail{}
	indices := map[uint64]int{}
	for rows.Next() {
		v := ClassDetail{Members: []Member{}, Teachers: []Teacher{}, Announcements: []Announcement{}}
		if err = rows.Scan(&v.ID, &v.AcademicYearID, &v.EducationLevelID, &v.MajorID, &v.GradeLevel, &v.Title, &v.Description, &v.Room, &v.Status, &v.CreatedBy, &v.Year, &v.Level, &v.Major, &v.MemberCount); err != nil {
			rows.Close()
			return nil, err
		}
		indices[v.ID] = len(items)
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if id != 0 && len(items) == 0 {
		return nil, sql.ErrNoRows
	}
	if summary || len(items) == 0 {
		return items, nil
	}
	if role != "student" {
		rows, err = q.QueryContext(ctx, `SELECT cm.class_id,u.id,u.login_id,u.full_name,u.status,p.nis FROM classes c JOIN class_members cm ON cm.class_id=c.id JOIN users u ON u.id=cm.student_user_id LEFT JOIN student_profiles p ON p.user_id=u.id WHERE `+scope+` AND cm.status='active' AND u.deleted_at IS NULL ORDER BY u.full_name,u.id`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var cid uint64
			var v Member
			if err = rows.Scan(&cid, &v.ID, &v.LoginID, &v.FullName, &v.Status, &v.NIS); err != nil {
				rows.Close()
				return nil, err
			}
			if i, ok := indices[cid]; ok {
				items[i].Members = append(items[i].Members, v)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		for i := range items {
			items[i].MemberCount = len(items[i].Members)
		}
	}
	rows, err = q.QueryContext(ctx, `SELECT ct.class_id,ct.id,u.id,u.full_name,ct.role,ct.subject_id,s.name,u.status FROM classes c JOIN class_teachers ct ON ct.class_id=c.id JOIN users u ON u.id=ct.teacher_user_id LEFT JOIN subjects s ON s.id=ct.subject_id WHERE `+scope+` AND ct.status='active' AND u.deleted_at IS NULL ORDER BY ct.role,u.full_name,ct.id`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var cid uint64
		var v Teacher
		if err = rows.Scan(&cid, &v.ID, &v.TeacherID, &v.FullName, &v.Role, &v.SubjectID, &v.SubjectName, &v.Status); err != nil {
			rows.Close()
			return nil, err
		}
		if i, ok := indices[cid]; ok {
			items[i].Teachers = append(items[i].Teachers, v)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.QueryContext(ctx, `SELECT a.class_id,a.id,a.title,a.content,u.full_name,DATE_FORMAT(a.created_at,'%Y-%m-%dT%H:%i:%sZ') FROM classes c JOIN announcements a ON a.class_id=c.id JOIN users u ON u.id=a.author_user_id WHERE `+scope+` AND a.deleted_at IS NULL ORDER BY a.created_at DESC,a.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var cid uint64
		var v Announcement
		if err = rows.Scan(&cid, &v.ID, &v.Title, &v.Content, &v.Author, &v.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if i, ok := indices[cid]; ok {
			items[i].Announcements = append(items[i].Announcements, v)
		}
	}
	err = rows.Err()
	rows.Close()
	return items, err
}

func (h *Handler) Workspace(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, 401, "Masuk ke portal dahulu.")
		return
	}
	items, err := h.service.workspace(r.Context(), claims.UserID, claims.Role, 0, r.URL.Query().Get("summary") == "true")
	if err != nil {
		classError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}
