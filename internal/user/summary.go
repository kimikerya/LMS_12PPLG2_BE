package user

import "net/http"

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	var out struct {
		Students int `json:"students"`
		Teachers int `json:"teachers"`
		Active   int `json:"active"`
	}
	err := h.service.repository.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(role='student'),0),COALESCE(SUM(role='teacher'),0),COALESCE(SUM(status='active'),0) FROM users WHERE deleted_at IS NULL`).Scan(&out.Students, &out.Teachers, &out.Active)
	if err != nil {
		managementError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
