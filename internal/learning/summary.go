package learning

import (
	"context"
	"net/http"
)

type ContentSummary struct {
	Total     int `json:"total"`
	Published int `json:"published"`
	Draft     int `json:"draft"`
}

func (s *Service) ContentSummary(ctx context.Context, a Actor) (map[string]ContentSummary, error) {
	if !monitoring(a.Role) {
		return nil, forbidden
	}
	out := make(map[string]ContentSummary, 3)
	for _, kind := range []string{"materials", "assignments", "assessments"} {
		var item ContentSummary
		// The table names are exclusively from the fixed list above.
		err := s.repository.q.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(status='published'),0),COALESCE(SUM(status='draft'),0) FROM "+kind+" WHERE deleted_at IS NULL").Scan(&item.Total, &item.Published, &item.Draft)
		if err != nil {
			return nil, err
		}
		out[kind] = item
	}
	return out, nil
}

func (h *Handler) ContentSummary(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.ContentSummary(r.Context(), actor(r))
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
