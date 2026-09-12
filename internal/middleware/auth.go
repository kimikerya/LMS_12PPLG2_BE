package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"lms-website-be/internal/auth"
)

type contextKey string

const claimsKey contextKey = "auth_claims"

func RequireAuth(service *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header := request.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(writer, http.StatusUnauthorized, "Bearer token wajib diisi")
			return
		}
		ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
		defer cancel()
		claims, err := service.Authenticate(ctx, strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			if errors.Is(err, auth.ErrUnauthorized) {
				writeError(writer, http.StatusUnauthorized, err.Error())
			} else {
				log.Printf("autentikasi: %v", err)
				writeError(writer, http.StatusServiceUnavailable, "layanan autentikasi belum tersedia")
			}
			return
		}
		ctx = context.WithValue(ctx, claimsKey, claims)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			claims, ok := ClaimsFromContext(request.Context())
			if !ok || !allowed[claims.Role] {
				writeError(writer, http.StatusForbidden, "role tidak memiliki izin")
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

func ClaimsFromContext(ctx context.Context) (auth.Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(auth.Claims)
	return claims, ok
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"error": message})
}
