package learning

import (
	"context"
	"math"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type Service struct{ repository *Repository }

func NewService(r *Repository) *Service { return &Service{repository: r} }
func (s *Service) List(ctx context.Context, a Actor, kind string, f Filter, id uint64) ([]Content, error) {
	if f.Limit == 0 {
		f.Limit = 50
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 {
		return nil, invalid("limit harus 1–100 dan offset tidak boleh negatif")
	}
	items, err := s.repository.List(ctx, a, kind, f, id)
	if err == nil && kind == "materials" && id != 0 && len(items) > 0 {
		items[0].Attachments, err = s.repository.MaterialAttachments(ctx, id)
	}
	if err == nil && kind == "assignments" && id != 0 && len(items) > 0 {
		items[0].Attachments, err = s.repository.AssignmentAttachments(ctx, id)
	}
	return items, err
}
func validTitle(title string) bool {
	return strings.TrimSpace(title) != "" && utf8.RuneCountInString(title) <= 200
}
func validURL(value *string) bool {
	if value == nil || len(*value) > 500 {
		return false
	}
	u, err := url.ParseRequestURI(*value)
	return err == nil && u.Hostname() != "" && u.User == nil && (u.Scheme == "http" || u.Scheme == "https")
}
func validPoints(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 9999.99 && math.Abs(v*100-math.Round(v*100)) < 0.000001
}

func (s *Service) CreateMaterial(ctx context.Context, a Actor, in MaterialInput) (uint64, error) {
	if a.Role != "teacher" {
		return 0, forbidden
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.ClassID == 0 || !validTitle(in.Title) {
		return 0, invalid("class_id dan title (maksimal 200 karakter) wajib diisi")
	}
	switch in.MaterialType {
	case "link", "video", "pdf", "document":
	default:
		return 0, invalid("material_type tidak valid")
	}
	if !validURL(in.URL) {
		return 0, invalid("isi url HTTP/HTTPS; upload file lokal belum tersedia")
	}
	if in.FilePath != nil {
		return 0, invalid("file_path belum didukung; gunakan url")
	}
	if in.MeetingNo != nil && *in.MeetingNo == 0 {
		return 0, invalid("meeting_no harus lebih dari nol")
	}
	return s.repository.create(ctx, a, in.ClassID, in.SubjectID, "materials", `INSERT INTO materials (class_id, teacher_user_id, subject_id, title, description, material_type, meeting_no, url) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, in.ClassID, a.ID, in.SubjectID, in.Title, in.Description, in.MaterialType, in.MeetingNo, in.URL)
}
func (s *Service) CreateAssignment(ctx context.Context, a Actor, in AssignmentInput) (uint64, error) {
	if a.Role != "teacher" {
		return 0, forbidden
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.ClassID == 0 || !validTitle(in.Title) || strings.TrimSpace(in.Instructions) == "" || in.DueAt.IsZero() {
		return 0, invalid("class_id, title, instructions dan due_at wajib diisi")
	}
	if in.CloseAt != nil && in.CloseAt.Before(in.DueAt) {
		return 0, invalid("close_at tidak boleh sebelum due_at")
	}
	points := 100.0
	if in.MaxPoints != nil {
		points = *in.MaxPoints
	}
	if !validPoints(points) || points == 0 {
		return 0, invalid("max_points harus 0.01–9999.99, maksimal dua desimal")
	}
	in.DueAt = in.DueAt.UTC()
	if in.CloseAt != nil {
		t := in.CloseAt.UTC()
		in.CloseAt = &t
	}
	return s.repository.create(ctx, a, in.ClassID, in.SubjectID, "assignments", `INSERT INTO assignments (class_id, teacher_user_id, subject_id, title, instructions, due_at, close_at, allow_late, max_points) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, in.ClassID, a.ID, in.SubjectID, in.Title, in.Instructions, in.DueAt, in.CloseAt, in.AllowLate, points)
}
func (s *Service) CreateAssessment(ctx context.Context, a Actor, in AssessmentInput) (uint64, error) {
	if a.Role != "teacher" {
		return 0, forbidden
	}
	in.Title = strings.TrimSpace(in.Title)
	if !validTitle(in.Title) || (in.AssessmentType != "quiz" && in.AssessmentType != "online_exam") {
		return 0, invalid("title atau assessment_type tidak valid")
	}
	if in.DurationMinutes != nil && *in.DurationMinutes == 0 {
		return 0, invalid("duration_minutes harus lebih dari nol")
	}
	if in.StartAt != nil && in.EndAt != nil && !in.EndAt.After(*in.StartAt) {
		return 0, invalid("end_at harus setelah start_at")
	}
	if in.StartAt != nil {
		t := in.StartAt.UTC()
		in.StartAt = &t
	}
	if in.EndAt != nil {
		t := in.EndAt.UTC()
		in.EndAt = &t
	}
	return s.repository.create(ctx, a, 0, in.SubjectID, "assessments", `INSERT INTO assessments (teacher_user_id, subject_id, title, assessment_type, description, instructions, duration_minutes, start_at, end_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, a.ID, in.SubjectID, in.Title, in.AssessmentType, in.Description, in.Instructions, in.DurationMinutes, in.StartAt, in.EndAt)
}
func (s *Service) Publish(ctx context.Context, a Actor, kind string, id uint64) error {
	if a.Role != "teacher" {
		return forbidden
	}
	return s.repository.Publish(ctx, a, kind, id)
}
func (s *Service) Submit(ctx context.Context, a Actor, id uint64, in SubmitInput) (uint64, error) {
	if a.Role != "student" {
		return 0, forbidden
	}
	switch in.SubmissionType {
	case "file":
		if len(in.Files) == 0 || len(in.Files) > 5 || in.LinkURL != nil || in.TextAnswer != nil && len(*in.TextAnswer) > 60000 {
			return 0, invalid("Unggah 1–5 file jawaban; catatan maksimal 60 KB.")
		}
	case "text":
		if in.TextAnswer == nil || strings.TrimSpace(*in.TextAnswer) == "" || len(*in.TextAnswer) > 60000 || in.LinkURL != nil {
			return 0, invalid("isi text_answer maksimal 60000 byte tanpa link_url")
		}
	case "link":
		if !validURL(in.LinkURL) || in.TextAnswer != nil {
			return 0, invalid("isi link_url HTTP/HTTPS tanpa text_answer")
		}
	default:
		return 0, invalid("Pilih jawaban teks, tautan, atau file.")
	}
	if in.SubmissionType != "file" && len(in.Files) > 0 {
		return 0, invalid("Gunakan bentuk jawaban file untuk unggahan.")
	}
	return s.repository.Submit(ctx, a, id, in)
}
func (s *Service) Submissions(ctx context.Context, a Actor, id uint64) ([]Submission, error) {
	items, err := s.repository.Submissions(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if err = s.repository.SubmissionFiles(ctx, id, items); err != nil {
		return nil, err
	}
	if a.Role == "student" {
		for i := range items {
			if items[i].ReleasedAt == nil {
				items[i].Score = nil
				items[i].Feedback = nil
				items[i].GradedAt = nil
				if items[i].Status == "graded" {
					items[i].Status = "submitted"
				}
			}
		}
	}
	return items, nil
}
func (s *Service) Grade(ctx context.Context, a Actor, id uint64, in GradeInput) error {
	if a.Role != "teacher" {
		return forbidden
	}
	if in.Score == nil || !validPoints(*in.Score) {
		return invalid("score wajib diisi, nonnegatif, maksimal dua desimal")
	}
	if in.Feedback != nil && len(*in.Feedback) > 60000 {
		return invalid("feedback terlalu panjang")
	}
	return s.repository.Grade(ctx, a, id, in, false)
}
func (s *Service) Release(ctx context.Context, a Actor, id uint64) error {
	if a.Role != "teacher" {
		return forbidden
	}
	return s.repository.Grade(ctx, a, id, GradeInput{}, true)
}

func submissionStatus(status string, due time.Time, closeAt *time.Time, lateAllowed bool, now time.Time) (string, error) {
	if status != "published" {
		return "", conflict("tugas belum diterbitkan atau sudah ditutup")
	}
	if closeAt != nil && !now.Before(*closeAt) {
		return "", conflict("batas akhir pengumpulan sudah lewat")
	}
	if now.After(due) {
		if !lateAllowed {
			return "", conflict("tenggat tugas sudah lewat")
		}
		return "late", nil
	}
	return "submitted", nil
}
