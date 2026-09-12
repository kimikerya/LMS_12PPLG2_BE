package classroom

import (
	"context"
	"lms-website-be/internal/testutil"
	"testing"
)

func TestMySQLClassManagement(t *testing.T) {
	f := testutil.MySQL(t)
	s := NewService(&Repository{db: f.Tx, transact: f.Transact})
	ctx := context.Background()
	options, err := s.options(ctx)
	if err != nil || len(options.Years) == 0 || len(options.Subjects) == 0 {
		t.Fatalf("options missing: %v", err)
	}
	detail, err := s.detail(ctx, f.Class)
	if err != nil || len(detail.Members) != 1 || len(detail.Teachers) != 1 {
		t.Fatalf("class detail incomplete: %v", err)
	}
	if err = s.announce(ctx, f.Class, f.Admin, AnnouncementInput{Title: "Info", Content: "School announcement"}); err != nil {
		t.Fatal(err)
	}
	detail, err = s.detail(ctx, f.Class)
	if err != nil || len(detail.Announcements) != 1 || detail.Announcements[0].Content != "School announcement" {
		t.Fatalf("announcement missing: %v", err)
	}
	other, err := s.detail(ctx, f.OtherClass)
	if err != nil || len(other.Announcements) != 0 {
		t.Fatalf("announcement crossed class: %v", err)
	}
	bad := CreateInput{Title: "Class", AcademicYearID: 0, EducationLevelID: detail.EducationLevelID, GradeLevel: 10}
	if err = validateClass(ctx, f.Tx, &bad); err == nil {
		t.Fatal("invalid reference accepted")
	}
	f.Exec(t, "UPDATE classes SET status='archived' WHERE id=?", f.Class)
	if err = s.announce(ctx, f.Class, f.Admin, AnnouncementInput{Title: "Hidden", Content: "No"}); err == nil {
		t.Fatal("archived class writable")
	}
}
