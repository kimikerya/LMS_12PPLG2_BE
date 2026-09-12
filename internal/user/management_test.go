package user

import (
	"context"
	"lms-website-be/internal/database"
	"lms-website-be/internal/testutil"
	"strings"
	"testing"
)

func TestUserValidation(t *testing.T) {
	base := CreateInput{LoginID: "user", Email: "user@example.test", FullName: "Test", Role: "teacher", Status: "active", Password: "TestOnly123!"}
	for _, change := range []func(*CreateInput){func(in *CreateInput) { in.Email = "not-email" }, func(in *CreateInput) { in.Password = strings.Repeat("a", 73) }, func(in *CreateInput) { in.Role = "student" }, func(in *CreateInput) { in.Role = "unknown" }} {
		in := base
		change(&in)
		if validateInput(&in, true) == nil {
			t.Fatal("invalid user accepted")
		}
	}
	in := base
	in.Password = ""
	if err := validateInput(&in, false); err != nil {
		t.Fatal(err)
	}
}
func TestMySQLImportAndProfileRollback(t *testing.T) {
	f := testutil.MySQL(t)
	ctx := context.Background()
	r := &Repository{db: f.Tx, transact: f.Transact}
	s := NewService(r)
	nis1, nis2 := f.Tag+"n1", f.Tag+"n2"
	in := []CreateInput{{LoginID: f.Tag + "import1", Email: f.Tag + "i1@example.test", FullName: "Import One", Role: "student", Password: "TestOnly123!", NIS: &nis1}, {LoginID: f.Tag + "import2", Email: f.Tag + "i2@example.test", FullName: "Import Two", Role: "student", Password: "TestOnly123!", NIS: &nis1}}
	if _, err := s.Import(ctx, in); err == nil {
		t.Fatal("duplicate NIS accepted")
	}
	var count int
	if err := f.Tx.QueryRow("SELECT COUNT(*) FROM users WHERE login_id IN (?,?)", in[0].LoginID, in[1].LoginID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial import remained: %d, %v", count, err)
	}
	in[1].NIS = &nis2
	if count, err := s.Import(ctx, in); err != nil || count != 2 {
		t.Fatalf("import failed: %d, %v", count, err)
	}
	var id uint64
	if err := f.Tx.QueryRow("SELECT id FROM users WHERE login_id=?", in[0].LoginID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	f.Exec(t, "UPDATE users SET avatar_url=?,bio=? WHERE id=?", "/avatar-test.png", "Profil siswa", id)
	detail, err := r.detail(ctx, id)
	if err != nil || detail.NIS == nil || *detail.NIS != nis1 || detail.AvatarURL == nil || *detail.AvatarURL != "/avatar-test.png" || detail.Bio == nil || *detail.Bio != "Profil siswa" {
		t.Fatalf("profile missing: %v", err)
	}
	update := in[0]
	update.FullName = "Changed"
	update.NIS = &nis2
	update.Status = "active"
	err = f.Transact(ctx, func(q database.Querier) error { return updateUser(ctx, q, id, f.Admin, update, "") })
	if err == nil {
		t.Fatal("duplicate profile update accepted")
	}
	detail, err = r.detail(ctx, id)
	if err != nil || detail.FullName != "Import One" {
		t.Fatalf("partial user edit persisted: %v", err)
	}
	update.NIS = &nis1
	update.Role = "teacher"
	if err = f.Transact(ctx, func(q database.Querier) error { return updateUser(ctx, q, id, f.Admin, update, "") }); err == nil {
		t.Fatal("role change accepted")
	}
	admin := CreateInput{LoginID: f.Tag + "a", Email: f.Tag + "a@example.test", FullName: "Admin", Role: "admin", Status: "inactive"}
	if err = f.Transact(ctx, func(q database.Querier) error { return updateUser(ctx, q, f.Admin, f.Admin, admin, "") }); err == nil {
		t.Fatal("self deactivation accepted")
	}
}
