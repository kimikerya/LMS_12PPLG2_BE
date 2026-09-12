package learning

import "time"

type Actor struct {
	ID   uint64
	Role string
}
type Problem struct {
	Status  int
	Message string
}

func (p *Problem) Error() string   { return p.Message }
func invalid(message string) error { return &Problem{422, message} }

var forbidden = &Problem{403, "Anda tidak memiliki izin untuk operasi ini"}
var notFound = &Problem{404, "data tidak ditemukan atau tidak dapat diakses"}

func conflict(message string) error { return &Problem{409, message} }

type Content struct {
	TeacherName      string     `json:"teacher_name"`
	MeetingNo        *uint16    `json:"meeting_no"`
	StartAt          *time.Time `json:"start_at"`
	EndAt            *time.Time `json:"end_at"`
	DurationMinutes  *uint16    `json:"duration_minutes"`
	Instructions     *string    `json:"instructions"`
	CloseAt          *time.Time `json:"close_at"`
	AllowLate        *bool      `json:"allow_late"`
	QuestionCount    *uint64    `json:"question_count"`
	SubmissionStatus *string    `json:"submission_status"`
	SubmittedAt      *time.Time `json:"submitted_at"`
	ID               uint64     `json:"id"`
	TeacherID        uint64     `json:"teacher_user_id"`
	ClassID          *uint64    `json:"class_id"`
	SubjectID        *uint64    `json:"subject_id"`
	Title            string     `json:"title"`
	Status           string     `json:"status"`
	PublishedAt      *time.Time `json:"published_at"`
	DueAt            *time.Time `json:"due_at"`
	Description      *string    `json:"description"`
	URL              *string    `json:"url"`
	FilePath         *string    `json:"file_path"`
	MaxPoints        *float64   `json:"max_points"`
	Type             *string    `json:"type"`
}
type Filter struct {
	Limit, Offset int
	ClassID       uint64
	SubjectID     uint64
}
type MaterialInput struct {
	ClassID      uint64  `json:"class_id"`
	SubjectID    *uint64 `json:"subject_id"`
	Title        string  `json:"title"`
	Description  *string `json:"description"`
	MaterialType string  `json:"material_type"`
	MeetingNo    *uint16 `json:"meeting_no"`
	URL          *string `json:"url"`
	FilePath     *string `json:"file_path"`
}
type AssignmentInput struct {
	ClassID      uint64     `json:"class_id"`
	SubjectID    *uint64    `json:"subject_id"`
	Title        string     `json:"title"`
	Instructions string     `json:"instructions"`
	DueAt        time.Time  `json:"due_at"`
	CloseAt      *time.Time `json:"close_at"`
	AllowLate    bool       `json:"allow_late"`
	MaxPoints    *float64   `json:"max_points"`
}
type AssessmentInput struct {
	SubjectID       *uint64    `json:"subject_id"`
	Title           string     `json:"title"`
	AssessmentType  string     `json:"assessment_type"`
	Description     *string    `json:"description"`
	Instructions    *string    `json:"instructions"`
	DurationMinutes *uint16    `json:"duration_minutes"`
	StartAt         *time.Time `json:"start_at"`
	EndAt           *time.Time `json:"end_at"`
}
type SubmitInput struct {
	SubmissionType string  `json:"submission_type"`
	TextAnswer     *string `json:"text_answer"`
	LinkURL        *string `json:"link_url"`
}
type GradeInput struct {
	Score    *float64 `json:"score"`
	Feedback *string  `json:"teacher_feedback"`
}
type Submission struct {
	ID           uint64     `json:"id"`
	AssignmentID uint64     `json:"assignment_id"`
	StudentID    uint64     `json:"student_user_id"`
	Type         string     `json:"submission_type"`
	TextAnswer   *string    `json:"text_answer"`
	LinkURL      *string    `json:"link_url"`
	SubmittedAt  *time.Time `json:"submitted_at"`
	Status       string     `json:"status"`
	Score        *float64   `json:"score"`
	Feedback     *string    `json:"teacher_feedback"`
	GradedAt     *time.Time `json:"graded_at"`
	ReleasedAt   *time.Time `json:"result_released_at"`
}

func monitoring(role string) bool {
	return role == "admin" || role == "curriculum" || role == "principal"
}
