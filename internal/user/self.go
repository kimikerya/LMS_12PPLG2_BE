package user

import (
	"lms-website-be/internal/middleware"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Self never accepts an ID from the caller. Academic identity remains admin-managed.
func (h *Handler) Self(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, 401, "Masuk ke portal dahulu.")
		return
	}
	if r.Method == http.MethodPatch {
		var in struct {
			FullName string `json:"full_name"`
			Bio      string `json:"bio"`
		}
		if !decodeManagement(w, r, &in) {
			return
		}
		in.FullName = strings.TrimSpace(in.FullName)
		in.Bio = strings.TrimSpace(in.Bio)
		if in.FullName == "" || utf8.RuneCountInString(in.FullName) > 150 || utf8.RuneCountInString(in.Bio) > 1000 {
			writeError(w, 422, "Nama wajib diisi, maksimal 150 karakter. Bio maksimal 1.000 karakter.")
			return
		}
		if _, err := h.service.repository.db.ExecContext(r.Context(), "UPDATE users SET full_name=?,bio=NULLIF(?,'') WHERE id=? AND deleted_at IS NULL", in.FullName, in.Bio, claims.UserID); err != nil {
			managementError(w, err)
			return
		}
	}
	item, err := h.service.repository.detail(r.Context(), claims.UserID)
	if err != nil {
		managementError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
