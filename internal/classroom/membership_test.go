package classroom

import (
	"context"
	"lms-website-be/internal/testutil"
	"testing"
)

func TestMembershipValidation(t *testing.T) {
	s := NewService(nil)
	if err := s.SetMembers(context.Background(), 1, 1, "student", MembersInput{UserIDs: []uint64{1}}); err == nil {
		t.Fatal("student allowed to enroll")
	}
	if err := s.SetMembers(context.Background(), 1, 1, "admin", MembersInput{UserIDs: []uint64{1, 1}}); err == nil {
		t.Fatal("duplicate ids accepted")
	}
	if err := s.SetTeacher(context.Background(), 1, 1, "admin", TeacherInput{TeacherID: 1, Role: "subject_teacher"}); err == nil {
		t.Fatal("missing subject accepted")
	}
}
func TestMySQLMembershipTransactions(t *testing.T) {
	f := testutil.MySQL(t)
	s := NewService(&Repository{db: f.Tx, transact: f.Transact})
	ctx := context.Background()
	// Student first, then a higher-id principal: failure must undo the first insert.
	err := s.SetMembers(ctx, f.Class, f.Admin, "admin", MembersInput{UserIDs: []uint64{f.OtherStudent, f.Principal}})
	if err == nil {
		t.Fatal("wrong role accepted")
	}
	var count int
	if err := f.Tx.QueryRow("SELECT COUNT(*) FROM class_members WHERE class_id=? AND student_user_id=?", f.Class, f.OtherStudent).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial batch persisted %d %v", count, err)
	}
	for i := 0; i < 2; i++ {
		if err := s.SetMembers(ctx, f.Class, f.Admin, "admin", MembersInput{UserIDs: []uint64{f.OtherStudent}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Tx.QueryRow("SELECT COUNT(*) FROM class_members WHERE class_id=? AND student_user_id=?", f.Class, f.OtherStudent).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate member %d %v", count, err)
	}
	if err := s.SetMembers(ctx, f.Class, f.Admin, "admin", MembersInput{UserIDs: []uint64{f.OtherStudent}, Status: "removed"}); err != nil {
		t.Fatal(err)
	}
	for _, teacher := range []uint64{f.Teacher, f.OtherTeacher, f.OtherTeacher} {
		if err := s.SetTeacher(ctx, f.Class, f.Admin, "admin", TeacherInput{TeacherID: teacher, Role: "homeroom"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Tx.QueryRow("SELECT COUNT(*) FROM class_teachers WHERE class_id=? AND role='homeroom' AND status='active'", f.Class).Scan(&count); err != nil || count != 1 {
		t.Fatalf("homeroom count=%d %v", count, err)
	}
	items, err := s.ListForUser(ctx, f.OtherTeacher, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("teacher class list duplicated: %d", len(items))
	}
	t.Log("batch rollback, idempotent enrollment, removal and single homeroom verified")
}
