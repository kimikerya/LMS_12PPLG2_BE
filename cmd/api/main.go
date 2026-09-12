package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"lms-website-be/internal/auth"
	"lms-website-be/internal/classroom"
	"lms-website-be/internal/config"
	"lms-website-be/internal/database"
	"lms-website-be/internal/learning"
	"lms-website-be/internal/middleware"
	"lms-website-be/internal/user"
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
	authService := auth.NewService(db, cfg.JWTSecret)
	userHandler := user.NewHandler(user.NewService(user.NewRepository(db)))
	classHandler := classroom.NewHandler(classroom.NewService(classroom.NewRepository(db)))
	learningHandler := learning.NewHandler(db)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"service": "SMK Citra Negara EMS API", "health": "/health/db", "frontend": "http://localhost:3000"})
	})
	mux.HandleFunc("POST /auth/login", func(writer http.ResponseWriter, request *http.Request) {
		var input struct {
			LoginID  string `json:"login_id"`
			Password string `json:"password"`
		}
		request.Body = http.MaxBytesReader(writer, request.Body, 8192)
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.LoginID == "" || input.Password == "" {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "login_id dan password wajib diisi"})
			return
		}
		ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
		defer cancel()
		result, err := authService.Login(ctx, input.LoginID, input.Password)
		if err != nil {
			if errors.Is(err, auth.ErrCredentials) {
				writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			} else {
				log.Printf("login: %v", err)
				writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": "layanan login belum tersedia"})
			}
			return
		}
		writeJSON(writer, http.StatusOK, result)
	})

	mux.Handle("GET /auth/me", middleware.RequireAuth(authService, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		claims, _ := middleware.ClaimsFromContext(request.Context())
		writeJSON(writer, http.StatusOK, claims)
	})))

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

	adminUsers := middleware.RequireAuth(authService, middleware.RequireRoles("admin")(
		http.HandlerFunc(userHandler.List),
	))
	mux.Handle("GET /api/profile", middleware.RequireAuth(authService, http.HandlerFunc(userHandler.Self)))
	mux.Handle("PATCH /api/profile", middleware.RequireAuth(authService, http.HandlerFunc(userHandler.Self)))
	mux.Handle("GET /api/users", adminUsers)
	mux.Handle("PATCH /api/users/{id}", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(http.HandlerFunc(userHandler.Update))))
	mux.Handle("POST /api/users/import", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(http.HandlerFunc(userHandler.Import))))
	mux.Handle("GET /api/academic-options", middleware.RequireAuth(authService, middleware.RequireRoles("admin", "teacher")(http.HandlerFunc(classHandler.Options))))
	mux.Handle("GET /api/classes/{id}", middleware.RequireAuth(authService, middleware.RequireRoles("admin", "teacher", "student")(http.HandlerFunc(classHandler.Detail))))
	mux.Handle("PATCH /api/classes/{id}", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(http.HandlerFunc(classHandler.Update))))
	mux.Handle("POST /api/classes/{id}/announcements", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(http.HandlerFunc(classHandler.Announce))))
	mux.Handle("POST /api/users", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(
		http.HandlerFunc(userHandler.Create),
	)))
	mux.Handle("GET /api/users/{id}", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(
		http.HandlerFunc(userHandler.Get),
	)))
	mux.Handle("PATCH /api/users/{id}/status", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(
		http.HandlerFunc(userHandler.UpdateStatus),
	)))
	mux.Handle("GET /api/classes", middleware.RequireAuth(authService, http.HandlerFunc(classHandler.List)))
	mux.Handle("POST /api/classes", middleware.RequireAuth(authService, middleware.RequireRoles("admin", "teacher")(
		http.HandlerFunc(classHandler.Create),
	)))
	mux.Handle("DELETE /api/users/{id}", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(http.HandlerFunc(userHandler.Delete))))
	mux.Handle("DELETE /api/classes/{id}", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(http.HandlerFunc(classHandler.Delete))))
	learningHandler.Register(mux, authService)
	mux.Handle("POST /api/classes/{id}/members", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(http.HandlerFunc(classHandler.Members))))
	mux.Handle("POST /api/classes/{id}/teachers", middleware.RequireAuth(authService, middleware.RequireRoles("admin")(http.HandlerFunc(classHandler.Teacher))))

	server := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Fatalf("API belum dapat berjalan pada port %s: %v. Hentikan server lama terlebih dahulu.", cfg.AppPort, err)
	}
	defer listener.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	log.Printf("API aktif di http://localhost:%s (PID %d). Ctrl+C untuk berhenti.", cfg.AppPort, os.Getpid())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-shutdownDone // Tunggu request aktif selesai sebelum koneksi database ditutup.
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
