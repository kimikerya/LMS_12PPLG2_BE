package learning

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func TestSubmissionDeadline(t *testing.T) {
	due := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, status string
		now          time.Time
		close        *time.Time
		late         bool
		want         string
	}{
		{"on time", "published", due.Add(-time.Minute), nil, false, "submitted"},
		{"at deadline", "published", due, nil, false, "submitted"},
		{"late blocked", "published", due.Add(time.Second), nil, false, ""},
		{"late allowed", "published", due.Add(time.Second), nil, true, "late"},
		{"close cutoff", "published", due.Add(time.Hour), ptr(due.Add(time.Hour)), true, ""},
		{"draft", "draft", due, nil, true, ""},
		{"closed", "closed", due, nil, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := submissionStatus(tc.status, due, tc.close, tc.late, tc.now)
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("got %s,%v", got, err)
			}
		})
	}
}
func TestServiceRejectsInvalidInputBeforeDatabase(t *testing.T) {
	s := NewService(nil)
	ctx := context.Background()
	_, err := s.CreateAssignment(ctx, Actor{1, "admin"}, AssignmentInput{})
	if !errors.Is(err, forbidden) {
		t.Fatal(err)
	}
	_, err = s.Submit(ctx, Actor{1, "student"}, 1, SubmitInput{SubmissionType: "link", LinkURL: ptr("javascript:alert(1)")})
	if err == nil {
		t.Fatal("unsafe URL accepted")
	}
	err = s.Grade(ctx, Actor{1, "teacher"}, 1, GradeInput{Score: ptr(1.001)})
	if err == nil {
		t.Fatal("fractional precision accepted")
	}
	_, err = s.CreateAssessment(ctx, Actor{1, "teacher"}, AssessmentInput{Title: "Quiz", AssessmentType: "invalid"})
	if err == nil {
		t.Fatal("invalid assessment accepted")
	}
}
func TestDecodeRejectsInjectedIdentityAndTrailingJSON(t *testing.T) {
	for _, body := range []string{`{"submission_type":"text","student_user_id":99}`, `{} {}`, strings.Repeat("x", 130*1024)} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var in SubmitInput
		if decode(w, r, &in) || w.Code != 400 {
			t.Fatalf("accepted invalid body, code %d", w.Code)
		}
	}
}
