package classroom

import (
	"context"
	"lms-website-be/internal/testutil"
	"testing"
)

func TestMySQLClassInvitesAndDeletion(t *testing.T) {
	f := testutil.MySQL(t)
	ctx := context.Background()
	s := NewService(&Repository{db: f.Tx, transact: f.Transact})
	invite, err := s.invite(ctx, f.Class, f.Teacher, "teacher", "POST")
	if err != nil || invite == nil {
		t.Fatalf("teacher invite: %v", err)
	}
	otherInvite, err := s.invite(ctx, f.OtherClass, f.Admin, "admin", "GET")
	if err != nil || otherInvite == nil || otherInvite.Code == invite.Code {
		t.Fatalf("class invite codes are not unique: %v", err)
	}
	if _, err = s.invite(ctx, f.Class, f.OtherTeacher, "teacher", "POST"); err == nil {
		t.Fatal("unassigned teacher created invite")
	}
	if _, err = s.invite(ctx, f.Class, f.Student, "student", "GET"); err == nil {
		t.Fatal("student read invite")
	}
	if _, err = s.join(ctx, f.OtherTeacher, "teacher", invite.Code); err == nil {
		t.Fatal("student code granted teacher access")
	}
	if err = classAccess(ctx, f.Tx, f.Class, f.OtherStudent, "student", false); err == nil {
		t.Fatal("nonmember read class")
	}
	if id, err := s.join(ctx, f.OtherStudent, "student", invite.Code); err != nil || id != f.Class {
		t.Fatalf("student join: %v", err)
	}
	if _, err = s.join(ctx, f.OtherStudent, "student", invite.Code); err != nil {
		t.Fatal("repeat join was not idempotent", err)
	}
	var count int
	f.Tx.QueryRow("SELECT COUNT(*) FROM class_members WHERE class_id=? AND student_user_id=?", f.Class, f.OtherStudent).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate membership")
	}
	if err = classAccess(ctx, f.Tx, f.Class, f.OtherStudent, "student", false); err != nil {
		t.Fatal(err)
	}
	next, err := s.invite(ctx, f.Class, f.Teacher, "teacher", "POST")
	if err != nil || next.Code != invite.Code {
		t.Fatal("persistent code changed", err)
	}
	if err = s.SetMembers(ctx, f.Class, f.Admin, "admin", MembersInput{UserIDs: []uint64{f.OtherStudent}, Status: "removed"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.join(ctx, f.OtherStudent, "student", invite.Code); err == nil {
		t.Fatal("removed member regained access")
	}
	f.Exec(t, "UPDATE class_invites SET expires_at=DATE_SUB(UTC_TIMESTAMP(),INTERVAL 1 SECOND) WHERE code=?", invite.Code)
	if _, err = s.join(ctx, f.Student, "student", invite.Code); err != nil {
		t.Fatal("persistent invite was rejected", err)
	}
	next, err = s.invite(ctx, f.Class, f.Admin, "admin", "POST")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.remove(ctx, f.Class, f.Admin); err != nil {
		t.Fatal(err)
	}
	if _, err = s.join(ctx, f.Student, "student", next.Code); err == nil {
		t.Fatal("deleted class join accepted")
	}
	f.Tx.QueryRow("SELECT COUNT(*) FROM class_members WHERE class_id=?", f.Class).Scan(&count)
	if count != 2 {
		t.Fatal("class history deleted")
	}
	classes, err := s.ListForUser(ctx, f.Admin, "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range classes {
		if c.ID == f.Class {
			t.Fatal("deleted class still listed")
		}
	}
}

func TestMySQLTeacherCreationAndDualAssignment(t *testing.T) {
	f := testutil.MySQL(t)
	ctx := context.Background()
	s := NewService(&Repository{db: f.Tx, transact: f.Transact})
	source, err := s.detail(ctx, f.Class)
	if err != nil {
		t.Fatal(err)
	}
	options, err := s.options(ctx)
	if err != nil {
		t.Fatal(err)
	}
	major := options.Majors[0].ID
	in := CreateInput{Title: f.Tag + "new", AcademicYearID: source.AcademicYearID, EducationLevelID: source.EducationLevelID, GradeLevel: 10, MajorID: &major, SubjectID: &f.Subject}
	missing := in
	missing.MajorID = nil
	if _, err = s.Create(ctx, missing, f.Admin, "admin"); err == nil {
		t.Fatal("missing required major accepted")
	}
	missing = in
	missing.SubjectID = nil
	if _, err = s.Create(ctx, missing, f.Teacher, "teacher"); err == nil {
		t.Fatal("teacher without subject accepted")
	}
	created, err := s.Create(ctx, in, f.Teacher, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.detail(ctx, created.ID)
	if err != nil || len(detail.Teachers) != 1 || detail.Teachers[0].TeacherID != f.Teacher {
		t.Fatal("creator not assigned atomically", err)
	}
	if _, err = s.Create(ctx, in, f.Student, "student"); err == nil {
		t.Fatal("student created class")
	}
	if err = s.SetTeacher(ctx, f.Class, f.Admin, "admin", TeacherInput{TeacherID: f.Teacher, Role: "homeroom", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetTeacher(ctx, f.Class, f.Admin, "admin", TeacherInput{TeacherID: f.Teacher, Role: "subject_teacher", SubjectID: &f.Subject, Status: "active"}); err != nil {
		t.Fatal(err)
	}
	detail, err = s.detail(ctx, f.Class)
	if err != nil || len(detail.Teachers) != 2 {
		t.Fatal("homeroom and subject must coexist", err)
	}
}
