package classroom

import (
	"context"
	"fmt"
	"strings"
)

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

func (s *Service) ListForUser(ctx context.Context, userID uint64, role string) ([]Class, error) {
	return s.repository.ListForUser(ctx, userID, role)
}

func (s *Service) Create(ctx context.Context, input CreateInput, createdBy uint64, role string) (Class, error) {
	if err := validateClass(ctx, s.repository.db, &input); err != nil {
		return Class{}, err
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.AcademicYearID == 0 || input.EducationLevelID == 0 || input.GradeLevel == 0 || input.Title == "" {
		return Class{}, fmt.Errorf("academic_year_id, education_level_id, grade_level, dan title wajib diisi")
	}
	return s.repository.Create(ctx, input, createdBy, role)
}
