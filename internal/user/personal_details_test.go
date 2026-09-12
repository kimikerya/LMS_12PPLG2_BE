package user

import (
	"context"
	"lms-website-be/internal/auth"
	"lms-website-be/internal/database"
	"lms-website-be/internal/testutil"
	"testing"
)

func TestPersonalDetailsValidation(t *testing.T) {
	base := CreateInput{LoginID: "profile", FullName: "Teacher", Role: "teacher", Password: "TestOnly123!"}
	if err := validateInput(&base, true); err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2023-02-29", "1899-12-31", "2999-01-01", "2000-1-2"} {
		in := base
		in.BirthDate = &date
		if validateInput(&in, true) == nil {
			t.Fatalf("invalid date accepted: %s", date)
		}
	}
	for _, phone := range []string{"hello", "123", "+12345678901234567890123456"} {
		in := base
		in.Phone = &phone
		if validateInput(&in, true) == nil {
			t.Fatalf("invalid phone accepted: %s", phone)
		}
	}
}

func TestMySQLPersonalDetailsAndEmailOptional(t *testing.T) {
	f := testutil.MySQL(t)
	ctx := context.Background()
	r := &Repository{db: f.Tx, transact: f.Transact}
	s := NewService(r)
	place, date, phone, employee := "Depok", "2000-02-29", "081234567890", f.Tag+"nip"
	in := CreateInput{LoginID: f.Tag + "profile", FullName: "Teacher", Role: "teacher", Password: "TestOnly123!", BirthPlace: &place, BirthDate: &date, Phone: &phone, EmployeeID: &employee}
	created, err := s.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	details, err := r.detail(ctx, created.ID)
	if err != nil || details.Email != "" || details.BirthDate == nil || *details.BirthDate != date || details.BirthPlace == nil || *details.BirthPlace != place || details.Phone == nil || *details.Phone != phone || details.EmployeeID == nil || *details.EmployeeID != employee {
		t.Fatalf("profile did not roundtrip: %+v, %v", details, err)
	}
	if _, err = auth.NewService(f.Tx, "test-secret").Login(ctx, in.LoginID, in.Password); err != nil {
		t.Fatalf("email-less login failed: %v", err)
	}
	other := CreateInput{LoginID: f.Tag + "other", FullName: "Other", Role: "admin", Password: "TestOnly123!"}
	if _, err = s.Import(ctx, []CreateInput{other}); err != nil {
		t.Fatalf("multiple empty emails must be allowed: %v", err)
	}
	in.BirthDate = nil
	in.BirthPlace = nil
	in.Phone = nil
	in.Status = "active"
	in.EmployeeID = nil
	if err = f.Transact(ctx, func(q database.Querier) error { return updateUser(ctx, q, created.ID, f.Admin, in, "") }); err != nil {
		t.Fatal(err)
	}
	details, err = r.detail(ctx, created.ID)
	if err != nil || details.BirthDate != nil || details.Phone != nil || details.EmployeeID != nil {
		t.Fatalf("optional fields were not cleared: %v", err)
	}
}
