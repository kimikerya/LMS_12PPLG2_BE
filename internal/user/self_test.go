package user

import (
	"context"
	"encoding/json"
	"lms-website-be/internal/auth"
	"lms-website-be/internal/middleware"
	"lms-website-be/internal/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMySQLSelfProfile(t *testing.T) {
	f := testutil.MySQL(t)
	h := NewHandler(NewService(&Repository{db: f.Tx, transact: f.Transact}))
	a := auth.NewService(f.Tx, "test-only-secret-long-enough-for-test")
	session, err := a.Login(context.Background(), f.Tag+"s", "TestOnly123!")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/profile", middleware.RequireAuth(a, http.HandlerFunc(h.Self)))
	mux.Handle("PATCH /api/profile", middleware.RequireAuth(a, http.HandlerFunc(h.Self)))
	call := func(method, body, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/profile", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("want %d got %d: %s", want, w.Code, w.Body.String())
		}
		return w
	}
	call("GET", "", "", 401)
	call("PATCH", `{"full_name":"Student Updated","bio":"Suka belajar"}`, session.Token, 200)
	w := call("GET", "", session.Token, 200)
	var out Detail
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.ID != f.Student || out.FullName != "Student Updated" || out.Bio == nil || *out.Bio != "Suka belajar" || out.NIS == nil {
		t.Fatal("own profile not returned correctly")
	}
	if strings.Contains(w.Body.String(), "password") {
		t.Fatal("password exposed")
	}
	call("PATCH", `{"full_name":"Wrong","bio":"","role":"admin"}`, session.Token, 400)
	call("PATCH", `{"full_name":"","bio":""}`, session.Token, 422)
	call("PATCH", `{"full_name":"Wrong","id":123}`, session.Token, 400)
	var name, role string
	if err := f.Tx.QueryRow("SELECT full_name,role FROM users WHERE id=?", f.Student).Scan(&name, &role); err != nil {
		t.Fatal(err)
	}
	if name != "Student Updated" || role != "student" {
		t.Fatal("rejected request modified profile")
	}
	if err := f.Tx.QueryRow("SELECT full_name FROM users WHERE id=?", f.OtherStudent).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Test s2" {
		t.Fatal("another student changed")
	}
}
