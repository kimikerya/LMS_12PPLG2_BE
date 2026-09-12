package learning

import (
	"context"
	"lms-website-be/internal/testutil"
	"net/http/httptest"
	"testing"
)

func TestSubjectFilterValidation(t *testing.T) {
	for _, value := range []string{"0", "-1", "abc", "1.5"} {
		if _, err := filter(httptest.NewRequest("GET", "/api/materials?subject_id="+value, nil)); err == nil {
			t.Fatalf("accepted invalid subject %s", value)
		}
	}
	f, err := filter(httptest.NewRequest("GET", "/api/materials?class_id=1&subject_id=12", nil))
	if err != nil || f.ClassID != 1 || f.SubjectID != 12 {
		t.Fatalf("filter=%+v err=%v", f, err)
	}
}

func TestMySQLSubjectContentIsolation(t *testing.T) {
	f := testutil.MySQL(t)
	repo := &Repository{q: f.Tx, transact: f.Transact}
	second := f.Insert(t, "INSERT INTO subjects(code,name) VALUES (?,?)", f.Tag+"2", "Second subject")
	for _, subject := range []uint64{f.Subject, second} {
		f.Insert(t, "INSERT INTO materials(class_id,teacher_user_id,subject_id,title,material_type,status) VALUES (?,?,?,?,'link','published')", f.Class, f.Teacher, subject, "Material")
		f.Insert(t, "INSERT INTO assignments(class_id,teacher_user_id,subject_id,title,instructions,due_at,status) VALUES (?,?,?,?,?,UTC_TIMESTAMP(),'published')", f.Class, f.Teacher, subject, "Task", "Instructions")
		assessment := f.Insert(t, "INSERT INTO assessments(teacher_user_id,subject_id,title,assessment_type,status) VALUES (?,?,?,'quiz','published')", f.Teacher, subject, "Assessment")
		f.Exec(t, "INSERT INTO assessment_targets(assessment_id,class_id) VALUES (?,?)", assessment, f.Class)
	}
	for _, kind := range []string{"materials", "assignments", "assessments"} {
		for _, subject := range []uint64{f.Subject, second} {
			items, err := repo.List(context.Background(), Actor{f.Student, "student"}, kind, Filter{Limit: 100, ClassID: f.Class, SubjectID: subject}, 0)
			if err != nil || len(items) != 1 || items[0].SubjectID == nil || *items[0].SubjectID != subject {
				t.Fatalf("%s subject=%d items=%+v err=%v", kind, subject, items, err)
			}
			items, err = repo.List(context.Background(), Actor{f.OtherStudent, "student"}, kind, Filter{Limit: 100, ClassID: f.Class, SubjectID: subject}, 0)
			if err != nil || len(items) != 0 {
				t.Fatalf("cross-class %s leaked: %+v err=%v", kind, items, err)
			}
		}
	}
}
