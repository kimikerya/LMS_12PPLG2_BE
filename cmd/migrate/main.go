package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"lms-website-be/internal/config"
	"lms-website-be/internal/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}

	db, err := database.Open(cfg)
	if err != nil {
		fail(err)
	}
	defer db.Close()

	if err := runMigrations(db, "migrations"); err != nil {
		fail(err)
	}

	fmt.Println("Migration selesai.")
}

func runMigrations(db *sql.DB, directory string) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) NOT NULL PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB`); err != nil {
		return fmt.Errorf("membuat schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("membaca folder migrations: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, file := range files {
		var exists bool
		if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", file).Scan(&exists); err != nil {
			return fmt.Errorf("memeriksa migration %s: %w", file, err)
		}
		if exists {
			fmt.Printf("Lewati %s (sudah diterapkan)\n", file)
			continue
		}

		sqlBytes, err := os.ReadFile(filepath.Join(directory, file))
		if err != nil {
			return fmt.Errorf("membaca migration %s: %w", file, err)
		}

		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			return fmt.Errorf("memulai transaction %s: %w", file, err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("menjalankan migration %s: %w", file, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", file); err != nil {
			tx.Rollback()
			return fmt.Errorf("mencatat migration %s: %w", file, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", file, err)
		}
		fmt.Printf("Terapkan %s\n", file)
	}

	return nil
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
