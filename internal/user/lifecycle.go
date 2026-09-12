package user

import (
	"context"
	"database/sql"
	"fmt"
	"lms-website-be/internal/database"
	"lms-website-be/internal/middleware"
	"net/http"
)

// Allocate inside the account transaction. Sequence locks serialize simultaneous imports.
func allocateLoginID(ctx context.Context, q database.Querier, role string) (string, error) {
	prefix := map[string]string{"student": "SIS", "teacher": "GUR", "admin": "ADM", "curriculum": "KUR", "principal": "KEP"}[role]
	if prefix == "" {
		return "", inputError("Peran tidak valid.")
	}
	for {
		if _, err := q.ExecContext(ctx, "INSERT INTO user_login_sequences(prefix,next_value) VALUES (?,1) ON DUPLICATE KEY UPDATE next_value=next_value+1", prefix); err != nil {
			return "", err
		}
		var next uint64
		if err := q.QueryRowContext(ctx, "SELECT next_value FROM user_login_sequences WHERE prefix=? FOR UPDATE", prefix).Scan(&next); err != nil {
			return "", err
		}
		login := fmt.Sprintf("%s-%06d", prefix, next)
		var exists bool
		if err := q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE login_id=?)", login).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return login, nil
		}
	}
}

func (s *Service) remove(ctx context.Context, id, actorID uint64) error {
	if id == actorID {
		return inputError("Akun yang sedang Anda gunakan tidak dapat dihapus.")
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		result, err := q.ExecContext(ctx, "UPDATE users SET deleted_at=UTC_TIMESTAMP(),status='inactive' WHERE id=? AND deleted_at IS NULL", id)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return sql.ErrNoRows
		}
		_, err = q.ExecContext(ctx, "INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id) VALUES (?,'user_deleted','users',?)", actorID, id)
		return err
	})
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "admin" {
		writeError(w, 403, "Akses khusus admin.")
		return
	}
	id, err := parseID(r)
	if err != nil || id == 0 {
		writeError(w, 400, "ID pengguna tidak valid.")
		return
	}
	if err = h.service.remove(r.Context(), id, claims.UserID); err != nil {
		managementError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
