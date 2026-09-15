package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"lms-website-be/internal/config"
	"lms-website-be/internal/database"
	"lms-website-be/internal/passwordpolicy"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Open(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	password := os.Getenv("SEED_ADMIN_PASSWORD")
	if password == "" {
		password = "Admin123!"
		log.Println("Peringatan: memakai password demo default Admin123!; hanya untuk database lokal.")
	}
	if err := seedAdmin(db, password); err != nil {
		log.Fatal(err)
	}
	log.Println("Seed admin selesai.")
}

func seedAdmin(db *sql.DB, password string) error {
	if !passwordpolicy.Valid(password) {
		return fmt.Errorf("%s", passwordpolicy.Message)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("membuat password hash: %w", err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("memulai transaction seed admin: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		INSERT INTO users (login_id, email, password_hash, full_name, role, status)
		VALUES ('ADMIN001', 'admin@example.test', ?, 'Administrator Demo', 'admin', 'active')
		ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)
	`, string(hash))
	if err != nil {
		return fmt.Errorf("membuat user admin: %w", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("mengambil id admin: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO staff_profiles (user_id, employee_id)
		VALUES (?, 'ADM001')
		ON DUPLICATE KEY UPDATE employee_id = VALUES(employee_id), updated_at = CURRENT_TIMESTAMP
	`, userID); err != nil {
		return fmt.Errorf("membuat profile admin: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed admin: %w", err)
	}
	return nil
}
