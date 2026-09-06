package database

import (
	"testing"

	"lms-website-be/internal/config"
)

func TestOpenRequiresReachableDatabase(t *testing.T) {
	cfg := config.Config{
		DBHost: "127.0.0.1",
		DBPort: "1",
		DBUser: "root",
		DBName: "smk_citra_ems",
	}

	db, err := Open(cfg)
	if err == nil {
		db.Close()
		t.Fatal("Open() seharusnya gagal pada port yang tidak aktif")
	}
}
