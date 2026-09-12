package database_test

import (
	"context"
	"errors"
	"lms-website-be/internal/database"
	"lms-website-be/internal/testutil"
	"testing"
)

func TestMySQLWithTxRollback(t *testing.T) {
	f := testutil.MySQL(t)
	want := errors.New("intentional failure")
	// Uses the actual production BeginTx/Rollback implementation on another connection.
	err := database.WithTx(context.Background(), f.DB, func(q database.Querier) error {
		_, err := q.ExecContext(context.Background(), `INSERT INTO users (login_id,email,password_hash,full_name,role,status) VALUES (?,?,'test-not-login','Rollback Test','student','inactive')`, f.Tag+"rollback", f.Tag+"rollback@example.test")
		if err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	var count int
	if err := f.DB.QueryRow("SELECT COUNT(*) FROM users WHERE login_id=?", f.Tag+"rollback").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback failed: %d %v", count, err)
	}
}
