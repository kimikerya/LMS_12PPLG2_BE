package user

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	f := ListFilter{Role: params.Get("role"), Status: params.Get("status"), Search: strings.TrimSpace(params.Get("q")), Limit: 25}
	page := 1
	var err error
	if raw := params.Get("page"); raw != "" {
		page, err = strconv.Atoi(raw)
	}
	if err != nil || page < 1 || page > 1000000 || utf8.RuneCountInString(f.Search) > 200 || f.Role != "" && !validRoles[f.Role] || f.Status != "" && !validStatuses[f.Status] {
		writeError(w, 422, "Filter pengguna tidak valid.")
		return
	}
	where, args := userFilter(f)
	var total int
	if err = h.service.repository.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users"+where, args...).Scan(&total); err != nil {
		managementError(w, err)
		return
	}
	last := (total + f.Limit - 1) / f.Limit
	if last < 1 {
		last = 1
	}
	if page > last {
		page = last
	}
	f.Offset = (page - 1) * f.Limit
	users, err := h.service.List(r.Context(), f)
	if err != nil {
		managementError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": users, "total": total, "page": page, "page_size": f.Limit})
}
