package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
)

func requestBudget(r *http.Request) time.Duration {
	if r.URL.Path == "/api/monitoring/export" {
		return 60 * time.Second
	}
	if strings.HasSuffix(r.URL.Path, "/upload") || strings.HasPrefix(r.URL.Path, "/api/learning-files/") {
		return 120 * time.Second
	}
	return 30 * time.Second
}

func RequestLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), requestBudget(r))
		defer cancel()
		deadline, _ := ctx.Deadline()
		// A canceled context alone cannot interrupt a slow request-body reader.
		_ = http.NewResponseController(w).SetReadDeadline(deadline)
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r.WithContext(ctx))
		if elapsed := time.Since(started); elapsed >= 2*time.Second {
			// Never log credentials, tokens, bodies or query strings.
			log.Printf("request lambat: %s %s durasi=%s", r.Method, r.URL.Path, elapsed.Round(time.Millisecond))
		}
	})
}
