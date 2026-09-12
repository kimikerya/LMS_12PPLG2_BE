package learning

import (
	"lms-website-be/internal/auth"
	"lms-website-be/internal/middleware"
	"net/http"
)

// Register keeps the production routes and integration tests on the same wiring.
func (h *Handler) Register(mux *http.ServeMux, authService *auth.Service) {
	protect := func(pattern string, fn http.HandlerFunc, roles ...string) {
		var next http.Handler = fn
		if len(roles) > 0 {
			next = middleware.RequireRoles(roles...)(next)
		}
		mux.Handle(pattern, middleware.RequireAuth(authService, next))
	}
	protect("GET /api/materials", h.ListMaterials)
	protect("GET /api/assignments", h.ListAssignments)
	protect("GET /api/assessments", h.ListAssessments)
	protect("POST /api/materials", h.CreateMaterial, "teacher")
	protect("POST /api/assignments", h.CreateAssignment, "teacher")
	protect("POST /api/assessments", h.CreateAssessment, "teacher")
	for _, kind := range []string{"materials", "assignments", "assessments"} {
		protect("GET /api/"+kind+"/{id}", h.Detail(kind))
	}
	for _, kind := range []string{"materials", "assignments"} {
		protect("POST /api/"+kind+"/{id}/publish", h.Publish(kind), "teacher")
	}
	protect("POST /api/assignments/{id}/submissions", h.Submit, "student")
	protect("GET /api/assignments/{id}/submissions", h.Submissions)
	protect("PATCH /api/submissions/{id}/grade", h.Grade, "teacher")
	protect("POST /api/submissions/{id}/release", h.Release, "teacher")
}
