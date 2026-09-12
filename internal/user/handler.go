package user

import (
	"context"
	"encoding/json"
	"lms-website-be/internal/middleware"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(writer http.ResponseWriter, request *http.Request) {
	users, err := h.service.List(request.Context(), ListFilter{Role: request.URL.Query().Get("role"), Status: request.URL.Query().Get("status")})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": users})
}

func (h *Handler) Get(writer http.ResponseWriter, request *http.Request) {
	id, err := parseID(request)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "id user tidak valid")
		return
	}
	item, err := h.service.repository.detail(request.Context(), id)
	if IsNotFound(err) {
		writeError(writer, http.StatusNotFound, "user tidak ditemukan")
		return
	}
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, item)
}

func (h *Handler) Create(writer http.ResponseWriter, request *http.Request) {
	var input CreateInput
	if !decodeManagement(writer, request, &input) {
		return
	}
	ctx, cancel := contextWithTimeout(request)
	defer cancel()
	item, err := h.service.Create(ctx, input)
	if err != nil {
		managementError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, item)
}

func (h *Handler) UpdateStatus(writer http.ResponseWriter, request *http.Request) {
	id, err := parseID(request)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "id user tidak valid")
		return
	}
	var input UpdateStatusInput
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "format JSON tidak valid")
		return
	}
	claims, ok := middleware.ClaimsFromContext(request.Context())
	if !ok || claims.Role != "admin" {
		writeError(writer, 403, "Akses khusus admin.")
		return
	}
	if id == claims.UserID && input.Status != "active" {
		writeError(writer, 422, "Anda tidak dapat menonaktifkan akun sendiri.")
		return
	}
	if err := h.service.UpdateStatus(request.Context(), id, input.Status); err != nil {
		if IsNotFound(err) {
			writeError(writer, http.StatusNotFound, "user tidak ditemukan")
			return
		}
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "updated"})
}

func parseID(request *http.Request) (uint64, error) {
	return strconv.ParseUint(request.PathValue("id"), 10, 64)
}

func contextWithTimeout(request *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(request.Context(), 5*time.Second)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": strings.TrimSpace(message)})
}
