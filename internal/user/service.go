package user

import (
	"context"
	"database/sql"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

var validRoles = map[string]bool{
	"student": true, "teacher": true, "admin": true, "curriculum": true, "principal": true,
}

var validStatuses = map[string]bool{
	"active": true, "inactive": true, "locked": true, "pending": true,
}

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]User, error) {
	return s.repository.List(ctx, filter)
}

func (s *Service) GetByID(ctx context.Context, id uint64) (User, error) {
	return s.repository.GetByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (User, error) {
	if err := validateInput(&input, true); err != nil {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("membuat password hash: %w", err)
	}
	return s.repository.CreateWithProfile(ctx, input, string(hash))
}

func (s *Service) UpdateStatus(ctx context.Context, id uint64, status string) error {
	if !validStatuses[status] {
		return inputError("status tidak valid")
	}
	return s.repository.UpdateStatus(ctx, id, status)
}

func IsNotFound(err error) bool {
	return err == sql.ErrNoRows
}
