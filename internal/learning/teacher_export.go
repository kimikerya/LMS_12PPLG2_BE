package learning

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

var teacherExportStatuses = map[string]bool{
	"": true, "submitted": true, "not_submitted": true, "late": true, "graded": true, "released": true,
}

func teacherGradeExportParams(r *http.Request) (TeacherGradeExportFilter, error) {
	var f TeacherGradeExportFilter
	var err error
	positive := func(key string, target *uint64, required bool) error {
		raw := r.URL.Query().Get(key)
		if raw == "" && !required {
			return nil
		}
		*target, err = strconv.ParseUint(raw, 10, 64)
		if err != nil || *target == 0 {
			return invalid(key + " tidak valid")
		}
		return nil
	}
	if err = positive("class_id", &f.ClassID, true); err != nil {
		return f, err
	}
	if err = positive("subject_id", &f.SubjectID, true); err != nil {
		return f, err
	}
	if err = positive("assignment_id", &f.AssignmentID, false); err != nil {
		return f, err
	}
	f.Status = r.URL.Query().Get("status")
	if !teacherExportStatuses[f.Status] {
		return f, invalid("status pengumpulan tidak valid")
	}
	parseDate := func(key string) (*time.Time, error) {
		raw := r.URL.Query().Get(key)
		if raw == "" {
			return nil, nil
		}
		v, e := time.ParseInLocation("2006-01-02", raw, schoolZone)
		if e != nil {
			return nil, invalid(key + " tidak valid")
		}
		return &v, nil
	}
	if f.From, err = parseDate("from"); err != nil {
		return f, err
	}
	if f.To, err = parseDate("to"); err != nil {
		return f, err
	}
	if (f.From == nil) != (f.To == nil) {
		return f, invalid("tanggal awal dan akhir harus diisi bersama")
	}
	if f.To != nil {
		end := f.To.AddDate(0, 0, 1)
		f.To = &end
		if !f.To.After(*f.From) || f.To.Sub(*f.From) > 366*24*time.Hour {
			return f, invalid("pilih rentang tanggal maksimal 366 hari")
		}
	}
	return f, nil
}

func (s *Service) TeacherGradeExport(ctx context.Context, a Actor, f TeacherGradeExportFilter) ([]TeacherGradeExportRow, error) {
	if a.Role != "teacher" {
		return nil, forbidden
	}
	// The assignment is intentionally not enough on its own: an active subject
	// teaching assignment is required for the requested class and subject.
	var allowed bool
	err := s.repository.q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM class_teachers ct JOIN classes c ON c.id=ct.class_id WHERE ct.class_id=? AND ct.subject_id=? AND ct.teacher_user_id=? AND ct.status='active' AND c.status='active')`, f.ClassID, f.SubjectID, a.ID).Scan(&allowed)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, forbidden
	}

	where := `a.class_id=? AND a.subject_id=? AND a.teacher_user_id=? AND a.deleted_at IS NULL`
	args := []any{f.ClassID, f.SubjectID, a.ID}
	if f.AssignmentID != 0 {
		where += " AND a.id=?"
		args = append(args, f.AssignmentID)
	}
	if f.From != nil {
		where += " AND COALESCE(a.published_at,a.created_at)>=? AND COALESCE(a.published_at,a.created_at)<?"
		args = append(args, f.From.UTC(), f.To.UTC())
	}
	// A roster row is emitted for every active learner who had joined when the
	// task was published. This keeps "not submitted" visible in the workbook.
	query := `SELECT u.full_name,COALESCE(sp.nis,u.login_id),c.title,sub.name,a.title,a.status,a.published_at,a.due_at,
		x.submitted_at,x.status,COALESCE(x.submitted_late,x.status='late',FALSE),x.score,COALESCE(a.max_points,100),x.graded_at,x.result_released_at
		FROM assignments a JOIN classes c ON c.id=a.class_id JOIN subjects sub ON sub.id=a.subject_id
		JOIN class_members cm ON cm.class_id=a.class_id AND cm.status='active' AND (a.published_at IS NULL OR cm.joined_at<=a.published_at)
		JOIN users u ON u.id=cm.student_user_id AND u.deleted_at IS NULL LEFT JOIN student_profiles sp ON sp.user_id=u.id
		LEFT JOIN assignment_submissions x ON x.assignment_id=a.id AND x.student_user_id=cm.student_user_id
		WHERE ` + where
	switch f.Status {
	case "submitted":
		query += " AND x.id IS NOT NULL"
	case "not_submitted":
		query += " AND x.id IS NULL"
	case "late":
		query += " AND x.id IS NOT NULL AND COALESCE(x.submitted_late,x.status='late',FALSE)"
	case "graded":
		query += " AND x.graded_at IS NOT NULL"
	case "released":
		query += " AND x.result_released_at IS NOT NULL"
	}
	query += " ORDER BY a.id DESC,u.full_name"
	rows, err := s.repository.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TeacherGradeExportRow{}
	for rows.Next() {
		v := TeacherGradeExportRow{}
		var submissionStatus sql.NullString
		if err := rows.Scan(&v.StudentName, &v.NIS, &v.ClassName, &v.SubjectName, &v.AssignmentTitle, &v.AssignmentStatus, &v.PublishedAt, &v.DueAt, &v.SubmittedAt, &submissionStatus, &v.Late, &v.Score, &v.MaxPoints, &v.GradedAt, &v.ReleasedAt); err != nil {
			return nil, err
		}
		if submissionStatus.Valid {
			v.SubmissionStatus = submissionStatus.String
		} else {
			v.SubmissionStatus = "not_submitted"
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if f.AssignmentID != 0 && len(out) == 0 {
		var exists bool
		err = s.repository.q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM assignments WHERE id=? AND class_id=? AND subject_id=? AND teacher_user_id=? AND deleted_at IS NULL)`, f.AssignmentID, f.ClassID, f.SubjectID, a.ID).Scan(&exists)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, notFound
		}
	}
	return out, nil
}

func teacherGradeWorkbook(rows []TeacherGradeExportRow, f TeacherGradeExportFilter) ([]byte, error) {
	info := [][]any{{"Keterangan", "Nilai"}, {"Kelas", fmt.Sprint(f.ClassID)}, {"Mata pelajaran", fmt.Sprint(f.SubjectID)}, {"Tugas", "Seluruh tugas"}, {"Periode", "Semua periode"}, {"Status pengumpulan", "Semua status"}, {"Dibuat pada WIB", time.Now().In(schoolZone).Format("2006-01-02 15:04")}}
	if f.AssignmentID != 0 {
		info[3][1] = fmt.Sprintf("Tugas #%d", f.AssignmentID)
	}
	if f.From != nil {
		info[4][1] = f.From.In(schoolZone).Format("2006-01-02") + " sampai " + f.To.Add(-time.Second).In(schoolZone).Format("2006-01-02")
	}
	if f.Status != "" {
		info[5][1] = f.Status
	}
	detail := [][]any{{"Nama siswa", "NIS / ID", "Kelas", "Mata pelajaran", "Tugas", "Status tugas", "Diterbitkan", "Tenggat", "Status pengumpulan", "Waktu pengumpulan", "Ketepatan", "Nilai", "Nilai maksimal", "Status penilaian", "Nilai dirilis"}}
	summary := [][]any{{"Nama siswa", "NIS / ID", "Total tugas", "Dikumpulkan", "Belum dikumpulkan", "Terlambat", "Sudah dinilai", "Rata-rata nilai (%)"}}
	type stat struct {
		name, nis                               string
		total, submitted, missing, late, graded int
		sum                                     float64
		scored                                  int
	}
	stats := map[string]*stat{}
	format := func(t *time.Time) string {
		if t == nil {
			return "—"
		}
		return t.In(schoolZone).Format("2006-01-02 15:04")
	}
	for _, v := range rows {
		collection := "Belum mengumpulkan"
		timing := "—"
		grading := "Belum dinilai"
		released := "Belum"
		if v.SubmissionStatus != "not_submitted" {
			collection = "Dikumpulkan"
			if v.Late {
				timing = "Terlambat"
			} else {
				timing = "Tepat waktu"
			}
		}
		if v.GradedAt != nil {
			grading = "Sudah dinilai"
		}
		if v.ReleasedAt != nil {
			released = "Ya"
		}
		var score any = "—"
		if v.Score != nil {
			score = *v.Score
		}
		detail = append(detail, []any{v.StudentName, v.NIS, v.ClassName, v.SubjectName, v.AssignmentTitle, v.AssignmentStatus, format(v.PublishedAt), format(v.DueAt), collection, format(v.SubmittedAt), timing, score, v.MaxPoints, grading, released})
		key := v.NIS + "/" + v.StudentName
		x := stats[key]
		if x == nil {
			x = &stat{name: v.StudentName, nis: v.NIS}
			stats[key] = x
		}
		x.total++
		if v.SubmissionStatus == "not_submitted" {
			x.missing++
		} else {
			x.submitted++
		}
		if v.Late {
			x.late++
		}
		if v.GradedAt != nil {
			x.graded++
		}
		if v.Score != nil && v.MaxPoints > 0 {
			x.sum += *v.Score / v.MaxPoints * 100
			x.scored++
		}
	}
	for _, v := range stats {
		var avg any = "—"
		if v.scored > 0 {
			avg = v.sum / float64(v.scored)
		}
		summary = append(summary, []any{v.name, v.nis, v.total, v.submitted, v.missing, v.late, v.graded, avg})
	}
	return makeWorkbook([]workbookSheet{{"Keterangan", info}, {"Rekap siswa", summary}, {"Rincian tugas", detail}})
}

func (h *Handler) TeacherGradeExport(w http.ResponseWriter, r *http.Request) {
	f, err := teacherGradeExportParams(r)
	if err != nil {
		respondError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rows, err := h.service.TeacherGradeExport(ctx, actor(r), f)
	if err != nil {
		respondError(w, err)
		return
	}
	data, err := teacherGradeWorkbook(rows, f)
	if err != nil {
		respondError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="rekap-nilai-guru.xlsx"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(data)
}
