package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"lms-website-be/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

// Open creates a MySQL connection pool and verifies that the server is reachable.
func Open(cfg config.Config) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci", cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("membuka koneksi database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("database tidak dapat dihubungi: %w", err)
	}

	return db, nil
}
