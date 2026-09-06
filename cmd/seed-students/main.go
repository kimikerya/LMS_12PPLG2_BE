package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"lms-website-be/internal/config"
	"lms-website-be/internal/database"

	"golang.org/x/crypto/bcrypt"
)

type student struct {
	loginID string
	email   string
	name    string
	nis     string
	nisn    string
}

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

	password := os.Getenv("SEED_STUDENT_PASSWORD")
	if password == "" {
		password = "Belajar123!"
		log.Println("Peringatan: memakai password demo default Belajar123!; hanya untuk database lokal.")
	}

	if err := seedStudents(db, password); err != nil {
		log.Fatal(err)
	}
	log.Println("Seed siswa selesai.")
}

func seedStudents(db *sql.DB, password string) error {
	students := []student{
		{loginID: "STD001", email: "student001@example.test", name: "Siswa Demo 1", nis: "20260001", nisn: "0060000001"},
		{loginID: "STD002", email: "student002@example.test", name: "Siswa Demo 2", nis: "20260002", nisn: "0060000002"},
		{loginID: "STD003", email: "student003@example.test", name: "Siswa Demo 3", nis: "20260003", nisn: "0060000003"},
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("membuat password hash: %w", err)
	}

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("memulai transaction seed: %w", err)
	}
	defer tx.Rollback()

	for _, item := range students {
		result, err := tx.Exec(`
			INSERT INTO users (login_id, email, password_hash, full_name, role, status)
			VALUES (?, ?, ?, ?, 'student', 'active')
			ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)
		`, item.loginID, item.email, string(hash), item.name)
		if err != nil {
			return fmt.Errorf("menambahkan user %s: %w", item.loginID, err)
		}
		userID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("mengambil id user %s: %w", item.loginID, err)
		}

		if _, err := tx.Exec(`
			INSERT INTO student_profiles (user_id, nis, nisn)
			VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE nis = VALUES(nis), nisn = VALUES(nisn), updated_at = CURRENT_TIMESTAMP
		`, userID, item.nis, item.nisn); err != nil {
			return fmt.Errorf("menambahkan profile %s: %w", item.loginID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed: %w", err)
	}

	return nil
}
