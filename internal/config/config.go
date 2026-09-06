package config

import (
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
}

// Load reads .env when it exists, then reads the environment variables.
// Existing system environment variables take precedence over .env values.
func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		AppPort:    getEnv("APP_PORT", "8080"),
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
	}

	if cfg.DBName == "" {
		return Config{}, fmt.Errorf("DB_NAME belum diatur")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
