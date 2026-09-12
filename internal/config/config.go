package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config contains the settings needed by the backend.
type Config struct {
	AppPort    string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	JWTSecret  string
}

// Load reads .env when it exists, then reads the environment variables.
// Existing system environment variables take precedence over .env values.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("file .env tidak dapat dibaca; periksa formatnya")
	}

	cfg := Config{
		AppPort:    getEnv("APP_PORT", "8080"),
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		JWTSecret:  os.Getenv("AUTH_JWT_SECRET"),
	}

	if cfg.DBName == "" {
		return Config{}, fmt.Errorf("DB_NAME belum diatur")
	}
	if len(cfg.JWTSecret) < 32 || cfg.JWTSecret == "local-development-secret-change-before-deployment" || cfg.JWTSecret == "replace-with-a-long-random-secret" {
		return Config{}, fmt.Errorf("AUTH_JWT_SECRET harus berisi secret acak minimal 32 byte, bukan nilai contoh")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
