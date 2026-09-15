package user

import (
	"context"
	"database/sql"
	"fmt"
	"lms-website-be/internal/database"
	"strings"
)

type Repository struct {
	db       database.Querier
	transact func(context.Context, func(database.Querier) error) error
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, transact: func(ctx context.Context, fn func(database.Querier) error) error { return database.WithTx(ctx, db, fn) }}
}

func userFilter(filter ListFilter) (string, []any) {
	query := " WHERE deleted_at IS NULL"
	args := make([]any, 0, 2)
	if filter.Role != "" {
		query += " AND role = ?"
		args = append(args, filter.Role)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.Search != "" {
		term := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(filter.Search) + "%"
		query += " AND (full_name LIKE ? ESCAPE '!' OR login_id LIKE ? ESCAPE '!' OR email LIKE ? ESCAPE '!')"
		args = append(args, term, term, term)
	}
	return query, args
}

func (r *Repository) List(ctx context.Context, filter ListFilter) ([]User, error) {
	where, args := userFilter(filter)
	query := `SELECT id, login_id, COALESCE(email,''), full_name, avatar_url, bio, role, status FROM users` + where + " ORDER BY id DESC"
	if filter.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, filter.Limit, filter.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mengambil daftar user: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var item User
		if err := rows.Scan(&item.ID, &item.LoginID, &item.Email, &item.FullName, &item.AvatarURL, &item.Bio, &item.Role, &item.Status); err != nil {
			return nil, fmt.Errorf("membaca daftar user: %w", err)
		}
		users = append(users, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("menyelesaikan daftar user: %w", err)
	}
	return users, nil
}

func (r *Repository) GetByID(ctx context.Context, id uint64) (User, error) {
	var item User
	err := r.db.QueryRowContext(ctx, `
		SELECT id, login_id, COALESCE(email,''), full_name, avatar_url, bio, role, status
		FROM users WHERE id = ? AND deleted_at IS NULL
	`, id).Scan(&item.ID, &item.LoginID, &item.Email, &item.FullName, &item.AvatarURL, &item.Bio, &item.Role, &item.Status)
	if err != nil {
		return User{}, err
	}
	return item, nil
}

func (r *Repository) CreateWithProfile(ctx context.Context, input CreateInput, passwordHash string) (User, error) {
	var id uint64
	err := r.transact(ctx, func(q database.Querier) error {
		var err error
		id, err = insertUser(ctx, q, input, passwordHash)
		return err
	})
	if err != nil {
		return User{}, err
	}
	return r.GetByID(ctx, id)
}

func insertUser(ctx context.Context, tx database.Querier, input CreateInput, passwordHash string) (uint64, error) {
	if input.LoginID == "" {
		login, err := allocateLoginID(ctx, tx, input.Role)
		if err != nil {
			return 0, err
		}
		input.LoginID = login
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO users (login_id, email, password_hash, full_name, role, status, birth_place, birth_date, phone)
		VALUES (?, NULLIF(?,''), ?, ?, ?, ?, ?, ?, ?)
	`, input.LoginID, input.Email, passwordHash, input.FullName, input.Role, input.Status, input.BirthPlace, input.BirthDate, input.Phone)
	if err != nil {
		return 0, err
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	switch input.Role {
	case "student":
		if input.NIS == nil || *input.NIS == "" {
			return 0, fmt.Errorf("NIS wajib untuk role student")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO student_profiles (user_id, nis, nisn) VALUES (?, ?, ?)`, userID, *input.NIS, input.NISN); err != nil {
			return 0, err
		}
	case "teacher":
		if _, err := tx.ExecContext(ctx, `INSERT INTO teacher_profiles (user_id, nik, nuptk, employee_id) VALUES (?, ?, ?, ?)`, userID, input.NIK, input.NUPTK, input.EmployeeID); err != nil {
			return 0, err
		}
	case "admin", "curriculum", "principal":
		if _, err := tx.ExecContext(ctx, `INSERT INTO staff_profiles (user_id, employee_id, nik, nuptk) VALUES (?, ?, ?, ?)`, userID, input.EmployeeID, input.NIK, input.NUPTK); err != nil {
			return 0, err
		}
	}

	return uint64(userID), nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id uint64, status string) error {
	result, err := r.db.ExecContext(ctx, "UPDATE users SET auth_version=auth_version+IF(status<>?,1,0), status = ? WHERE id = ? AND deleted_at IS NULL", status, status, id)
	if err != nil {
		return fmt.Errorf("memperbarui status user: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("memeriksa perubahan status user: %w", err)
	}
	if count == 0 {
		var exists bool
		if err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = ? AND deleted_at IS NULL)", id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return sql.ErrNoRows
		}
	}
	return nil
}
