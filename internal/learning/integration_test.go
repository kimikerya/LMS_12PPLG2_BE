package learning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lms-website-be/internal/auth"
	"lms-website-be/internal/database"
	"lms-website-be/internal/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMySQLLearningWorkflow(t *testing.T) {
	f := testutil.MySQL(t)
	s := NewService(&Repository{q: f.Tx, transact: f.Transact})
	authService := auth.NewService(f.Tx, "test-only-secret-long-enough-for-test")
	mux := http.NewServeMux()
	NewServiceHandler(s).Register(mux, authService)
	tokens := map[string]string{}
	for _, suffix := range []string{"a", "t", "t2", "s", "s2", "p", "c"} {
		result, err := authService.Login(context.Background(), f.Tag+suffix, "TestOnly123!")
		if err != nil {
			t.Fatal(err)
		}
		tokens[suffix] = result.Token
	}
	call := func(who, method, path string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		raw := ""
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			raw = string(b)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if who != "" {
			r.Header.Set("Authorization", "Bearer "+tokens[who])
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s as %s: want %d got %d %s", method, path, who, want, w.Code, w.Body.String())
		}
		return w
	}
	newID := func(w *httptest.ResponseRecorder) uint64 {
		var x struct {
			ID uint64 `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &x); err != nil {
			t.Fatal(err)
		}
		return x.ID
	}
	assignment := AssignmentInput{ClassID: f.Class, SubjectID: &f.Subject, Title: f.Tag + " task", Instructions: "Tuliskan jawaban", DueAt: time.Now().UTC().Add(time.Hour), MaxPoints: ptr(100.0)}
	call("", "GET", "/api/assignments", nil, 401)
	call("a", "POST", "/api/assignments", assignment, 403)
	call("t2", "POST", "/api/assignments", assignment, 403)
	id := newID(call("t", "POST", "/api/assignments", assignment, 201))
	path := fmt.Sprintf("/api/assignments/%d", id)
	call("s", "GET", path, nil, 404)
	call("t2", "POST", path+"/publish", nil, 403)
	call("t", "POST", path+"/publish", nil, 200)
	call("t", "POST", path+"/publish", nil, 200)
	call("s", "GET", path, nil, 200)
	call("s2", "GET", path, nil, 404)
	call("p", "GET", path, nil, 200)
	call("c", "GET", path, nil, 200)
	// Class filters do not bypass student membership.
	w := call("s2", "GET", fmt.Sprintf("/api/assignments?class_id=%d", f.Class), nil, 200)
	if strings.Contains(w.Body.String(), assignment.Title) {
		t.Fatal("cross-class list leak")
	}
	in := SubmitInput{SubmissionType: "text", TextAnswer: ptr("Jawaban siswa")}
	call("s2", "POST", path+"/submissions", in, 403)
	subID := newID(call("s", "POST", path+"/submissions", in, 201))
	call("s", "POST", path+"/submissions", in, 409)
	sub := fmt.Sprintf("/api/submissions/%d", subID)
	call("t", "POST", sub+"/release", nil, 409)
	grade := GradeInput{Score: ptr(85.0), Feedback: ptr("Bagus")}
	call("a", "PATCH", sub+"/grade", grade, 403)
	call("t2", "PATCH", sub+"/grade", grade, 403)
	call("t", "PATCH", sub+"/grade", GradeInput{Score: ptr(101.0)}, 422)
	call("t", "PATCH", sub+"/grade", grade, 200)
	w = call("s", "GET", path+"/submissions", nil, 200)
	var results struct {
		Data []Submission `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if len(results.Data) != 1 || results.Data[0].Score != nil || results.Data[0].Feedback != nil || results.Data[0].GradedAt != nil || results.Data[0].Status == "graded" {
		t.Fatal("unreleased grade leaked")
	}
	var studentTask Content
	if err := json.Unmarshal(call("s", "GET", path, nil, 200).Body.Bytes(), &studentTask); err != nil {
		t.Fatal(err)
	}
	if studentTask.TeacherName != "Test t" || studentTask.SubmissionStatus == nil || *studentTask.SubmissionStatus != "submitted" || studentTask.SubmittedAt == nil {
		t.Fatal("student task summary missing or unreleased grading state leaked")
	}
	call("s2", "GET", path+"/submissions", nil, 403)
	call("t", "POST", sub+"/release", nil, 200)
	call("t", "POST", sub+"/release", nil, 200)
	w = call("s", "GET", path+"/submissions", nil, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if results.Data[0].Score == nil || *results.Data[0].Score != 85 {
		t.Fatal("released grade missing")
	}
	if err := json.Unmarshal(call("s", "GET", path, nil, 200).Body.Bytes(), &studentTask); err != nil {
		t.Fatal(err)
	}
	if studentTask.SubmissionStatus == nil || *studentTask.SubmissionStatus != "graded" {
		t.Fatal("released grading state missing from task summary")
	}
	call("t", "PATCH", sub+"/grade", grade, 409)
	var count int
	if err := f.Tx.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ?", f.Student).Scan(&count); err != nil || count != 1 {
		t.Fatalf("notification count=%d err=%v", count, err)
	}
	// Removed membership and disabled accounts take effect with existing JWTs.
	f.Exec(t, "UPDATE class_members SET status='removed' WHERE class_id=? AND student_user_id=?", f.Class, f.Student)
	call("s", "GET", path, nil, 404)
	call("s", "GET", path+"/submissions", nil, 403)
	f.Exec(t, "UPDATE users SET status='inactive' WHERE id=?", f.Student)
	call("s", "GET", "/api/assignments", nil, 401)
	f.Exec(t, "UPDATE users SET role='student' WHERE id=?", f.Admin)
	call("a", "GET", path, nil, 404)
	// Material drafts/ownership use the same protected query paths.
	mat := MaterialInput{ClassID: f.Class, SubjectID: &f.Subject, Title: f.Tag + " material", MaterialType: "link", URL: ptr("https://example.test/material")}
	call("t2", "POST", "/api/materials", mat, 403)
	mid := newID(call("t", "POST", "/api/materials", mat, 201))
	call("t2", "POST", fmt.Sprintf("/api/materials/%d/publish", mid), nil, 403)
	call("t", "POST", fmt.Sprintf("/api/materials/%d/publish", mid), nil, 200)
	call("s2", "GET", fmt.Sprintf("/api/materials/%d", mid), nil, 404)
	ass := AssessmentInput{Title: f.Tag + " quiz", AssessmentType: "quiz"}
	aid := newID(call("t", "POST", "/api/assessments", ass, 201))
	// Domain regression: an ulangan belongs to assessments, never assignments.
	for _, tc := range []struct{ endpoint, present, absent string }{
		{"/api/assignments", assignment.Title, ass.Title},
		{"/api/assessments", ass.Title, assignment.Title},
	} {
		body := call("t", "GET", tc.endpoint, nil, 200).Body.String()
		if !strings.Contains(body, tc.present) || strings.Contains(body, tc.absent) {
			t.Fatalf("tugas and ulangan are mixed or missing from %s", tc.endpoint)
		}
	}
	call("s2", "GET", fmt.Sprintf("/api/assessments/%d", aid), nil, 404)
	call("t2", "GET", fmt.Sprintf("/api/assessments/%d", aid), nil, 404)
	// Even published assessments need a matching class target.
	f.Exec(t, "UPDATE assessments SET status='published' WHERE id=?", aid)
	f.Exec(t, "INSERT INTO assessment_targets (assessment_id,class_id) VALUES (?,?)", aid, f.Class)
	call("s2", "GET", fmt.Sprintf("/api/assessments/%d", aid), nil, 404)
	// A failure after inserting a submission rolls back the answer too.
	f.Exec(t, "UPDATE users SET status='active' WHERE id=?", f.Student)
	f.Exec(t, "UPDATE class_members SET status='active' WHERE class_id=? AND student_user_id=?", f.Class, f.Student)
	id2 := newID(call("t", "POST", "/api/assignments", assignment, 201))
	call("t", "POST", fmt.Sprintf("/api/assignments/%d/publish", id2), nil, 200)
	rollbackService := NewService(&Repository{q: f.Tx, transact: func(ctx context.Context, fn func(database.Querier) error) error {
		return f.Transact(ctx, func(q database.Querier) error {
			if err := fn(q); err != nil {
				return err
			}
			return errors.New("injected failure before commit")
		})
	}})
	_, err := rollbackService.Submit(context.Background(), Actor{f.Student, "student"}, id2, in)
	if err == nil {
		t.Fatal("expected transaction failure")
	}
	if err := f.Tx.QueryRow("SELECT COUNT(*) FROM assignment_submissions WHERE assignment_id=?", id2).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback count=%d err=%v", count, err)
	}
	t.Log("JWT, ownership, membership, publish, submit, grade, release, notification and rollback verified; fixtures roll back")
}
