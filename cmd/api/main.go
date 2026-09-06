package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"lms-website-be/internal/config"
	"lms-website-be/internal/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/db", func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()

		var result int
		if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
			writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"status": "database_unavailable"})
			return
		}

		writeJSON(writer, http.StatusOK, map[string]any{"status": "ok", "database": result})
	})

	server := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("API berjalan di http://localhost:%s", cfg.AppPort)
	log.Fatal(server.ListenAndServe())
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
