// Command backup creates, verifies and restores local LMS backups.
// Restore always creates a NEW database and never replaces an existing one.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"lms-website-be/internal/backup"
	"lms-website-be/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Backup/restore gagal:", err)
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "backup", "backup, verify, atau restore")
	source := flag.String("source", "", "Folder backup untuk verify/restore")
	output := flag.String("output", ".local/backups", "Folder induk backup")
	tools := flag.String("mysql-bin", "", "Folder mysql dan mysqldump bila tidak ada di PATH")
	target := flag.String("target-db", "", "Database BARU tujuan restore; tidak boleh sudah ada")
	flag.Parse()
	if *mode == "verify" {
		if *source == "" {
			return fmt.Errorf("isi -source")
		}
		manifest, err := backup.Verify(*source)
		if err == nil {
			fmt.Printf("Backup valid: %d file, dibuat %s.\n", len(manifest.Files), manifest.Created.Format(time.RFC3339))
		}
		return err
	}
	if *mode != "backup" && *mode != "restore" {
		return fmt.Errorf("mode tidak dikenal")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	credentials, cleanup, err := credentialFile(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if *mode == "backup" {
		return create(ctx, cfg, credentials, *tools, *output)
	}
	if *source == "" || *target == "" {
		return fmt.Errorf("restore memerlukan -source dan -target-db")
	}
	return restore(ctx, cfg, credentials, *tools, *source, *target)
}

// Password is supplied in a private temporary option file, never in argv/logs.
func credentialFile(cfg config.Config) (string, func(), error) {
	file, err := os.CreateTemp("", "lms-mysql-*.cnf")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	escape := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r")
	_, err = fmt.Fprintf(file, "[client]\nhost=\"%s\"\nport=\"%s\"\nuser=\"%s\"\npassword=\"%s\"\n", escape.Replace(cfg.DBHost), escape.Replace(cfg.DBPort), escape.Replace(cfg.DBUser), escape.Replace(cfg.DBPassword))
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return file.Name(), cleanup, nil
}

func executable(directory, name string) (string, error) {
	if directory != "" {
		for _, suffix := range []string{"", ".exe"} {
			candidate := filepath.Join(directory, name+suffix)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	matches, _ := filepath.Glob("C:/laragon/bin/mysql/*/bin/" + name + ".exe")
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", fmt.Errorf("%s belum ditemukan; isi -mysql-bin dengan folder bin MySQL", name)
}

func create(ctx context.Context, cfg config.Config, credentials, tools, output string) error {
	dump, err := executable(tools, "mysqldump")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(output, 0700); err != nil {
		return err
	}
	directory, err := os.MkdirTemp(output, "backup-"+time.Now().UTC().Format("20060102-150405")+"-")
	if err != nil {
		return err
	}
	// An unfinished directory has no manifest and cannot pass verify/restore.
	args := []string{"--defaults-extra-file=" + credentials, "--single-transaction", "--quick", "--skip-lock-tables", "--no-tablespaces", "--hex-blob", "--result-file=" + filepath.Join(directory, "database.sql"), cfg.DBName}
	command := exec.CommandContext(ctx, dump, args...)
	if err = command.Run(); err != nil {
		return fmt.Errorf("mysqldump gagal (%v); periksa koneksi dan izin database. Backup belum lengkap: %s", err, directory)
	}
	uploads := os.Getenv("LMS_UPLOAD_DIR")
	if uploads == "" {
		uploads = filepath.Join(".local", "uploads")
	}
	if _, err = os.Stat(uploads); os.IsNotExist(err) {
		err = os.Mkdir(filepath.Join(directory, "uploads"), 0700)
	} else if err == nil {
		err = backup.CopyTree(uploads, filepath.Join(directory, "uploads"))
	}
	if err != nil {
		return err
	}
	if err = backup.Seal(directory, cfg.DBName); err != nil {
		return err
	}
	if _, err = backup.Verify(directory); err != nil {
		return err
	}
	abs, _ := filepath.Abs(directory)
	fmt.Printf("Backup selesai dan checksum valid: %s\n", abs)
	return nil
}

func restore(ctx context.Context, cfg config.Config, credentials, tools, source, target string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`).MatchString(target) || strings.EqualFold(target, cfg.DBName) {
		return fmt.Errorf("nama database tujuan harus baru, berbeda dari DB_NAME, dan hanya huruf/angka/underscore")
	}
	if _, err := backup.Verify(source); err != nil {
		return err
	}
	client, err := executable(tools, "mysql")
	if err != nil {
		return err
	}
	dsn := mysql.NewConfig()
	dsn.User = cfg.DBUser
	dsn.Passwd = cfg.DBPassword
	dsn.Net = "tcp"
	dsn.Addr = cfg.DBHost + ":" + cfg.DBPort
	dsn.Timeout = 5 * time.Second
	db, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	// No IF NOT EXISTS: even an empty existing database must be protected.
	if _, err = db.ExecContext(ctx, "CREATE DATABASE `"+target+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return fmt.Errorf("database tujuan tidak dapat dibuat; mungkin sudah ada atau izin tidak cukup")
	}
	sqlFile, err := os.Open(filepath.Join(source, "database.sql"))
	if err != nil {
		return err
	}
	defer sqlFile.Close()
	command := exec.CommandContext(ctx, client, "--defaults-extra-file="+credentials, "--binary-mode", "--database="+target)
	command.Stdin = sqlFile
	if err = command.Run(); err != nil {
		return fmt.Errorf("impor gagal; database baru %s dibiarkan untuk pemeriksaan, database LMS asli tidak diubah", target)
	}
	parent := filepath.Join(".local", "restores")
	if err = os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	directory, err := os.MkdirTemp(parent, target+"-")
	if err != nil {
		return err
	}
	uploads := filepath.Join(directory, "uploads")
	if err = backup.CopyTree(filepath.Join(source, "uploads"), uploads); err != nil {
		return err
	}
	if err = checkRestoredUploads(ctx, db, target, uploads); err != nil {
		return err
	}
	var count int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", target).Scan(&count); err != nil {
		return err
	}
	abs, _ := filepath.Abs(uploads)
	fmt.Printf("Restore selesai ke database baru %s (%d tabel).\nFile: %s\nKonfigurasi LMS aktif belum diubah.\n", target, count, abs)
	return nil
}

// Validate database-to-file references too: checksums alone cannot detect files
// that were already missing from the source server when backup was taken.
func checkRestoredUploads(ctx context.Context, db *sql.DB, target, directory string) error {
	var missing int
	for _, table := range []string{"assignment_attachments", "submission_files", "material_attachments"} {
		var exists int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name=?", target, table).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			continue
		}
		rows, err := db.QueryContext(ctx, "SELECT DISTINCT file_url FROM `"+target+"`.`"+table+"` WHERE file_url LIKE 'local:%'")
		if err != nil {
			return err
		}
		for rows.Next() {
			var stored string
			if err = rows.Scan(&stored); err != nil {
				rows.Close()
				return err
			}
			key := strings.TrimPrefix(stored, "local:")
			if !regexp.MustCompile(`^[a-fA-F0-9]{32}$`).MatchString(key) {
				missing++
				continue
			}
			info, statErr := os.Stat(filepath.Join(directory, key))
			if statErr != nil || !info.Mode().IsRegular() {
				missing++
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	if missing > 0 {
		return fmt.Errorf("restore database selesai tetapi %d referensi file tidak tersedia; jangan gunakan hasil ini sebagai LMS aktif", missing)
	}
	return nil
}
