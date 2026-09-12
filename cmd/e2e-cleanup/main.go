// Removes only records bearing an exact, generated E2E tag from a local database.
// Used by the frontend browser suite after its create/edit/import workflow.
package main

import (
	"context"
	"flag"
	"lms-website-be/internal/config"
	"lms-website-be/internal/database"
	"log"
	"regexp"
	"time"
)

func main() {
	tag := flag.String("tag", "", "generated E2E tag")
	flag.Parse()
	if !regexp.MustCompile(`^E2E[0-9a-f]{16,40}$`).MatchString(*tag) {
		log.Fatal("Invalid E2E tag")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Cannot load test configuration")
	}
	if cfg.DBHost != "localhost" && cfg.DBHost != "127.0.0.1" && cfg.DBHost != "::1" {
		log.Fatal("E2E cleanup requires a local database")
	}
	db, err := database.Open(cfg)
	if err != nil {
		log.Fatal("Cannot connect to local test database")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = database.WithTx(ctx, db, func(q database.Querier) error {
		// Match the exact random tag and separator; never a free-form LIKE pattern.
		prefix := *tag + "-%"
		for _, query := range []string{
			"DELETE FROM audit_logs WHERE entity_type='classes' AND entity_id IN (SELECT id FROM classes WHERE title LIKE ?)",
			"DELETE FROM classes WHERE title LIKE ?",
			"DELETE FROM audit_logs WHERE entity_type='users' AND entity_id IN (SELECT id FROM users WHERE login_id LIKE ?)",
			"DELETE FROM audit_logs WHERE entity_type='users' AND entity_id IN (SELECT id FROM users WHERE full_name LIKE ?)",
			"DELETE FROM users WHERE login_id LIKE ?",
			"DELETE FROM users WHERE full_name LIKE ?",
		} {
			if _, err := q.ExecContext(ctx, query, prefix); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal("E2E cleanup failed; inspect only records with tag " + *tag)
	}
}
