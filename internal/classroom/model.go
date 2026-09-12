package classroom

type Class struct {
	ID               uint64  `json:"id"`
	AcademicYearID   uint64  `json:"academic_year_id"`
	EducationLevelID uint64  `json:"education_level_id"`
	MajorID          *uint64 `json:"major_id,omitempty"`
	GradeLevel       uint8   `json:"grade_level"`
	Title            string  `json:"title"`
	Description      *string `json:"description,omitempty"`
	Room             *string `json:"room,omitempty"`
	Status           string  `json:"status"`
	CreatedBy        uint64  `json:"created_by"`
}

type CreateInput struct {
	SubjectID        *uint64 `json:"subject_id"`
	AcademicYearID   uint64  `json:"academic_year_id"`
	EducationLevelID uint64  `json:"education_level_id"`
	MajorID          *uint64 `json:"major_id"`
	GradeLevel       uint8   `json:"grade_level"`
	Title            string  `json:"title"`
	Description      *string `json:"description"`
	Room             *string `json:"room"`
}
