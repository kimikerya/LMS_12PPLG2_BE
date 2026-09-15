package classroom

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"lms-website-be/internal/middleware"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(writer http.ResponseWriter, request *http.Request) {
	claims, ok := middleware.ClaimsFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, "autentikasi diperlukan")
		return
	}
	items, err := h.service.ListForUser(request.Context(), claims.UserID, claims.Role)
	if err != nil {
		classError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) Create(writer http.ResponseWriter, request *http.Request) {
	claims, ok := middleware.ClaimsFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, "autentikasi diperlukan")
		return
	}
	var input CreateInput
	if !classDecode(writer, request, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	item, err := h.service.Create(ctx, input, claims.UserID, claims.Role)
	if err != nil {
		classError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, item)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}
