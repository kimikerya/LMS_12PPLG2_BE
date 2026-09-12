package user

import (
	"context"
	"lms-website-be/internal/auth"
	"lms-website-be/internal/testutil"
	"testing"
)

func TestMySQLAutomaticLoginAndSoftDeletion(t *testing.T) {
	f := testutil.MySQL(t)
	ctx := context.Background()
	r := &Repository{db: f.Tx, transact: f.Transact}
	s := NewService(r)
	in := CreateInput{FullName: f.Tag + "Auto", Role: "teacher", Password: "TestOnly123!"}
	a, err := s.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(ctx, in)
	if err != nil || a.LoginID == "" || a.LoginID == b.LoginID {
		t.Fatal("automatic IDs must be unique", err)
	}
	authService := auth.NewService(f.Tx, "test-secret")
	login, err := authService.Login(ctx, a.LoginID, in.Password)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.remove(ctx, f.Admin, f.Admin); err == nil {
		t.Fatal("self delete accepted")
	}
	if err = s.remove(ctx, a.ID, f.Admin); err != nil {
		t.Fatal(err)
	}
	if _, err = authService.Authenticate(ctx, login.Token); err == nil {
		t.Fatal("deleted user token still accepted")
	}
	if _, err = authService.Login(ctx, a.LoginID, in.Password); err == nil {
		t.Fatal("deleted user login accepted")
	}
	if _, err = r.detail(ctx, a.ID); err == nil {
		t.Fatal("deleted profile exposed")
	}
	if err = s.UpdateStatus(ctx, a.ID, "active"); err == nil {
		t.Fatal("deleted user reactivated through status")
	}
	var count int
	f.Tx.QueryRow("SELECT COUNT(*) FROM teacher_profiles WHERE user_id=?", a.ID).Scan(&count)
	if count != 1 {
		t.Fatal("profile history lost")
	}
	all, err := s.List(ctx, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range all {
		if u.ID == a.ID {
			t.Fatal("deleted user listed")
		}
	}
}
