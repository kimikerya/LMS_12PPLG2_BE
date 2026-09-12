package classroom

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"lms-website-be/internal/database"
	"lms-website-be/internal/middleware"
	"net/http"
	"regexp"
	"strings"
)

type Invite struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expires_at"`
}

func randomInviteCode() (string, error) {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(raw[:])), nil
}

func ensureInvite(ctx context.Context, q database.Querier, classID, actor uint64) (*Invite, error) {
	var value Invite
	err := q.QueryRowContext(ctx, "SELECT code,COALESCE(DATE_FORMAT(expires_at,'%Y-%m-%dT%H:%i:%sZ'),'') FROM class_invites WHERE class_id=? AND is_active=TRUE ORDER BY id DESC LIMIT 1", classID).Scan(&value.Code, &value.ExpiresAt)
	if err == nil {
		// Existing codes are now permanent. This also upgrades a legacy code
		// that still has an expiration timestamp.
		if value.ExpiresAt != "" {
			if _, err = q.ExecContext(ctx, "UPDATE class_invites SET expires_at=NULL WHERE class_id=? AND code=?", classID, value.Code); err != nil {
				return nil, err
			}
			value.ExpiresAt = ""
		}
		return &value, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, err = q.ExecContext(ctx, "UPDATE class_invites SET is_active=FALSE WHERE class_id=? AND is_active=TRUE", classID); err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 8; attempt++ {
		code, err := randomInviteCode()
		if err != nil {
			return nil, err
		}
		var exists bool
		if err = q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM class_invites WHERE code=?)", code).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			continue
		}
		if _, err = q.ExecContext(ctx, "INSERT INTO class_invites(class_id,code,created_by,expires_at) VALUES (?,?,?,NULL)", classID, code, actor); err != nil {
			// The unique index remains the final guard if two transactions
			// generate the same code at the same time.
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				continue
			}
			return nil, err
		}
		return &Invite{Code: code}, nil
	}
	return nil, errors.New("gagal membuat kode kelas unik")
}

func (s *Service) invite(ctx context.Context, id, actor uint64, role, operation string) (*Invite, error) {
	var out *Invite
	err := s.repository.transact(ctx, func(q database.Querier) error {
		if err := activeClass(ctx, q, id); err != nil {
			return err
		}
		if err := classAccess(ctx, q, id, actor, role, true); err != nil {
			return err
		}
		if operation == "DELETE" {
			if _, err := q.ExecContext(ctx, "UPDATE class_invites SET is_active=FALSE WHERE class_id=?", id); err != nil {
				return err
			}
			return nil
		}
		var inviteErr error
		out, inviteErr = ensureInvite(ctx, q, id, actor)
		return inviteErr
	})
	return out, err
}
func (h *Handler) Invite(w http.ResponseWriter, r *http.Request) {
	id, ok := classID(w, r)
	if !ok {
		return
	}
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, 401, "Masuk ke portal dahulu.")
		return
	}
	out, err := h.service.invite(r.Context(), id, claims.UserID, claims.Role, r.Method)
	if err != nil {
		classError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"invite": out})
}

func (s *Service) join(ctx context.Context, actor uint64, role, code string) (uint64, error) {
	if role != "student" {
		return 0, accessError("Kode bergabung hanya untuk akun siswa.")
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if !regexp.MustCompile(`^[A-F0-9]{12}$`).MatchString(code) {
		return 0, membershipError("Kode kelas tidak valid atau sudah kedaluwarsa.")
	}
	var classID uint64
	err := s.repository.transact(ctx, func(q database.Querier) error {
		// Read the class first, then lock it in the same order as deletion.
		if err := q.QueryRowContext(ctx, "SELECT class_id FROM class_invites WHERE code=?", code).Scan(&classID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return membershipError("Kode kelas tidak valid atau sudah kedaluwarsa.")
			}
			return err
		}
		if err := activeClass(ctx, q, classID); err != nil {
			return err
		}
		var valid bool
		if err := q.QueryRowContext(ctx, "SELECT is_active FROM class_invites WHERE code=? FOR UPDATE", code).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return membershipError("Kode kelas tidak valid atau sudah kedaluwarsa.")
		}
		if err := roleCheck(ctx, q, actor, "student", true); err != nil {
			return err
		}
		var status string
		err := q.QueryRowContext(ctx, "SELECT status FROM class_members WHERE class_id=? AND student_user_id=? FOR UPDATE", classID, actor).Scan(&status)
		if err == nil {
			if status == "removed" {
				return membershipError("Keanggotaan Anda telah dikeluarkan. Hubungi admin untuk bergabung kembali.")
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err = q.ExecContext(ctx, "INSERT INTO class_members(class_id,student_user_id,joined_via) VALUES (?,?,'code')", classID, actor); err != nil {
			return err
		}
		_, err = q.ExecContext(ctx, "INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id) VALUES (?,'student_joined_code','classes',?)", actor, classID)
		return err
	})
	return classID, err
}
func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, 401, "Masuk ke portal dahulu.")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if !classDecode(w, r, &in) {
		return
	}
	id, err := h.service.join(r.Context(), claims.UserID, claims.Role, in.Code)
	if err != nil {
		classError(w, err)
		return
	}
	writeJSON(w, 200, map[string]uint64{"class_id": id})
}

func (s *Service) remove(ctx context.Context, id, actor uint64) error {
	return s.repository.transact(ctx, func(q database.Querier) error {
		if err := activeClass(ctx, q, id); err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx, "UPDATE classes SET status='archived' WHERE id=?", id); err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx, "UPDATE class_invites SET is_active=FALSE WHERE class_id=?", id); err != nil {
			return err
		}
		_, err := q.ExecContext(ctx, "INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id) VALUES (?,'class_deleted','classes',?)", actor, id)
		return err
	})
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := classID(w, r)
	if !ok {
		return
	}
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "admin" {
		writeError(w, 403, "Akses khusus admin.")
		return
	}
	if err := h.service.remove(r.Context(), id, claims.UserID); err != nil {
		classError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
