// Package testutil provides rollback-only fixtures for opt-in MySQL tests.
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"lms-website-be/internal/config"
	"lms-website-be/internal/database"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

type Fixture struct {
	DB                                                                         *sql.DB
	Tx                                                                         *sql.Tx
	Tag                                                                        string
	Admin, Teacher, OtherTeacher, Student, OtherStudent, Principal, Curriculum uint64
	Class, OtherClass, Subject                                                 uint64
}

func MySQL(t *testing.T) *Fixture {
	t.Helper()
	if os.Getenv("LMS_INTEGRATION") != "1" {
		t.Skip("set LMS_INTEGRATION=1 untuk test MySQL lokal")
	}
	_, file, _, _ := runtime.Caller(0)
	if err := godotenv.Load(filepath.Join(filepath.Dir(file), "..", "..", ".env")); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBHost != "127.0.0.1" && cfg.DBHost != "localhost" && cfg.DBHost != "::1" {
		t.Fatal("integration test hanya untuk MySQL lokal")
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	f := &Fixture{DB: db, Tx: tx, Tag: "T" + strconv.FormatInt(time.Now().UnixNano(), 36)}
	t.Cleanup(func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			t.Error(err)
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE login_id LIKE ?", f.Tag+"%").Scan(&count); err != nil {
			t.Error(err)
		} else if count != 0 {
			t.Errorf("fixture masih tersimpan: %d", count)
		}
	})
	hash, err := bcrypt.GenerateFromPassword([]byte("TestOnly123!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(suffix, role string) uint64 {
		login := f.Tag + suffix
		id := f.Insert(t, `INSERT INTO users (login_id,email,password_hash,full_name,role,status) VALUES (?,?,?,?,?,'active')`, login, login+"@example.test", string(hash), "Test "+suffix, role)
		switch role {
		case "student":
			f.Exec(t, "INSERT INTO student_profiles (user_id,nis) VALUES (?,?)", id, login)
		case "teacher":
			f.Exec(t, "INSERT INTO teacher_profiles (user_id) VALUES (?)", id)
		default:
			f.Exec(t, "INSERT INTO staff_profiles (user_id) VALUES (?)", id)
		}
		return id
	}
	f.Admin = makeUser("a", "admin")
	f.Teacher = makeUser("t", "teacher")
	f.OtherTeacher = makeUser("t2", "teacher")
	f.Student = makeUser("s", "student")
	f.OtherStudent = makeUser("s2", "student")
	f.Principal = makeUser("p", "principal")
	f.Curriculum = makeUser("c", "curriculum")
	year := f.Insert(t, "INSERT INTO academic_years (name) VALUES (?)", f.Tag)
	level := f.Insert(t, "INSERT INTO education_levels (name) VALUES (?)", f.Tag)
	f.Subject = f.Insert(t, "INSERT INTO subjects (code,name) VALUES (?,?)", f.Tag, "Test Subject")
	f.Class = f.Insert(t, `INSERT INTO classes (academic_year_id,education_level_id,grade_level,title,created_by) VALUES (?,?,10,?,?)`, year, level, f.Tag+" C1", f.Admin)
	f.OtherClass = f.Insert(t, `INSERT INTO classes (academic_year_id,education_level_id,grade_level,title,created_by) VALUES (?,?,10,?,?)`, year, level, f.Tag+" C2", f.Admin)
	f.Exec(t, "INSERT INTO class_members (class_id,student_user_id) VALUES (?,?),(?,?)", f.Class, f.Student, f.OtherClass, f.OtherStudent)
	f.Exec(t, "INSERT INTO class_teachers (class_id,teacher_user_id,subject_id,role) VALUES (?,?,?,'subject_teacher'),(?,?,?,'subject_teacher')", f.Class, f.Teacher, f.Subject, f.OtherClass, f.OtherTeacher, f.Subject)
	return f
}
func (f *Fixture) Insert(t *testing.T, q string, args ...any) uint64 {
	t.Helper()
	res, err := f.Tx.Exec(q, args...)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return uint64(id)
}
func (f *Fixture) Exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := f.Tx.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

// Savepoints isolate each service operation while the outer fixture is always rolled back.
func (f *Fixture) Transact(ctx context.Context, fn func(database.Querier) error) error {
	if _, err := f.Tx.ExecContext(ctx, "SAVEPOINT test_operation"); err != nil {
		return err
	}
	err := fn(f.Tx)
	if err != nil {
		if _, rollbackErr := f.Tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT test_operation"); rollbackErr != nil {
			return fmt.Errorf("%v; rollback: %w", err, rollbackErr)
		}
	}
	_, releaseErr := f.Tx.ExecContext(ctx, "RELEASE SAVEPOINT test_operation")
	if err != nil {
		return err
	}
	return releaseErr
}
