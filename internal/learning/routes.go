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
	protect("GET /api/learning/attention", h.Attention, "student", "teacher")
	protect("POST /api/materials/upload", h.UploadMaterial, "teacher")
	protect("POST /api/materials/{id}/upload", h.UploadMaterial, "teacher")
	protect("GET /api/materials/{id}/readers", h.MaterialReaders, "teacher", "curriculum", "principal")
	protect("GET /api/learning-files/material/{id}", h.DownloadFile("material"))
	protect("GET /api/learning/summary", h.ContentSummary, "curriculum", "principal")
	protect("GET /api/monitoring/report", h.Monitoring, "curriculum", "principal")
	protect("GET /api/monitoring/export", h.MonitoringExport, "curriculum", "principal")
	protect("POST /api/monitoring/reviews", h.MonitorReview, "curriculum")
	protect("POST /api/materials/{id}/access", h.MaterialAccess, "student")
	protect("GET /api/assignments", h.ListAssignments)
	protect("GET /api/assessments", h.ListAssessments)
	protect("POST /api/materials", h.CreateMaterial, "teacher")
	protect("PATCH /api/materials/{id}", h.EditMaterial, "teacher")
	protect("POST /api/assignments/{id}/upload", h.EditAssignmentUpload, "teacher")
	protect("POST /api/assignments", h.CreateAssignment, "teacher")
	protect("POST /api/assignments/upload", h.UploadAssignment, "teacher")
	protect("POST /api/assignments/{id}/submissions/upload", h.UploadSubmission, "student")
	protect("GET /api/learning-files/assignment/{id}", h.DownloadFile("assignment"))
	protect("GET /api/learning-files/submission/{id}", h.DownloadFile("submission"))
	protect("POST /api/assessments", h.CreateAssessment, "teacher")
	protect("POST /api/assessments/drafts", h.SaveAssessment, "teacher")
	protect("GET /api/assessments/{id}/editor", h.AssessmentEditor, "teacher")
	protect("PUT /api/assessments/{id}/editor", h.SaveAssessment, "teacher")
	protect("POST /api/assessments/{id}/publish", h.PublishAssessment, "teacher")
	protect("POST /api/assessments/{id}/start", h.StartExam, "student")
	protect("PUT /api/assessment-attempts/{id}/answers", h.SaveExam(false), "student")
	protect("POST /api/assessment-attempts/{id}/submit", h.SaveExam(true), "student")
	protect("GET /api/assessments/{id}/attempts", h.ExamAttempts, "teacher")
	protect("PATCH /api/assessment-attempts/{id}/grade", h.GradeExam(false), "teacher")
	protect("POST /api/assessment-attempts/{id}/release", h.GradeExam(true), "teacher")
	for _, kind := range []string{"materials", "assignments", "assessments"} {
		protect("GET /api/"+kind+"/{id}", h.Detail(kind))
		protect("DELETE /api/"+kind+"/{id}", h.DeleteContent(kind), "teacher")
	}
	for _, kind := range []string{"materials", "assignments"} {
		protect("POST /api/"+kind+"/{id}/publish", h.Publish(kind), "teacher")
	}
	protect("POST /api/assignments/{id}/submissions", h.Submit, "student")
	protect("GET /api/assignments/{id}/submissions", h.Submissions)
	protect("GET /api/assessments/{id}/results", h.AssessmentResults, "student")
	protect("PATCH /api/submissions/{id}/grade", h.Grade, "teacher")
	protect("POST /api/submissions/{id}/release", h.Release, "teacher")
}
