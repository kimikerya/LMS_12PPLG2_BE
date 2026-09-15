package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"io"
	"lms-website-be/internal/database"
	"lms-website-be/internal/middleware"
	"lms-website-be/internal/passwordpolicy"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type inputError string

func (e inputError) Error() string { return string(e) }

func validateInput(in *CreateInput, create bool) error {
	in.LoginID = strings.TrimSpace(in.LoginID)
	in.FullName = strings.TrimSpace(in.FullName)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Status == "" {
		in.Status = "active"
	}
	if (!create && in.LoginID == "") || utf8.RuneCountInString(in.LoginID) > 100 || in.FullName == "" || utf8.RuneCountInString(in.FullName) > 150 {
		return inputError("Nama lengkap dan ID pengguna wajib diisi sesuai batas panjang.")
	}
	if in.Email != "" {
		address, err := mail.ParseAddress(in.Email)
		if err != nil || address.Address != in.Email || len(in.Email) > 255 {
			return inputError("Alamat email tidak valid.")
		}
	}
	for _, field := range []struct {
		value **string
		limit int
	}{{&in.BirthPlace, 100}, {&in.BirthDate, 10}, {&in.Phone, 25}} {
		if *field.value != nil {
			value := strings.TrimSpace(**field.value)
			if utf8.RuneCountInString(value) > field.limit {
				return inputError("Data pribadi melebihi batas panjang.")
			}
			if value == "" {
				*field.value = nil
			} else {
				*field.value = &value
			}
		}
	}
	if in.BirthDate != nil {
		date, err := time.Parse("2006-01-02", *in.BirthDate)
		if err != nil || date.Year() < 1900 || date.Format("2006-01-02") > time.Now().UTC().Format("2006-01-02") {
			return inputError("Tanggal lahir harus valid, mulai tahun 1900 dan tidak di masa depan.")
		}
	}
	if in.Phone != nil && !regexp.MustCompile(`^\+?[0-9][0-9 ()-]{6,23}[0-9]$`).MatchString(*in.Phone) {
		return inputError("Nomor HP tidak valid (8-25 karakter).")
	}
	if !validRoles[in.Role] || !validStatuses[in.Status] {
		return inputError("Peran atau status tidak valid.")
	}
	if (create || in.Password != "") && !passwordpolicy.Valid(in.Password) {
		return inputError(passwordpolicy.Message)
	}
	for _, p := range []**string{&in.NIS, &in.NISN, &in.NIK, &in.NUPTK, &in.EmployeeID} {
		if *p != nil {
			value := strings.TrimSpace(**p)
			if utf8.RuneCountInString(value) > 50 {
				return inputError("Nomor identitas maksimal 50 karakter.")
			}
			if value == "" {
				*p = nil
			} else {
				*p = &value
			}
		}
	}
	if in.Role == "student" && in.NIS == nil {
		return inputError("NIS wajib diisi untuk siswa.")
	}
	return nil
}

func managementError(w http.ResponseWriter, err error) {
	var invalid inputError
	var dbErr *mysql.MySQLError
	switch {
	case errors.As(err, &invalid):
		writeError(w, 422, invalid.Error())
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, 404, "Pengguna tidak ditemukan.")
	case errors.As(err, &dbErr) && dbErr.Number == 1062:
		writeError(w, 409, "ID pengguna, email, atau nomor identitas sudah digunakan. Periksa data Anda.")
	default:
		log.Printf("user management: %v", err)
		writeError(w, 500, "Data pengguna belum dapat disimpan. Silakan coba kembali.")
	}
}
func decodeManagement(w http.ResponseWriter, r *http.Request, in any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 262144)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(in) != nil || d.Decode(&struct{}{}) != io.EOF {
		writeError(w, 400, "Kirim satu objek JSON yang valid.")
		return false
	}
	return true
}

type Detail struct {
	BirthPlace *string `json:"birth_place"`
	BirthDate  *string `json:"birth_date"`
	Phone      *string `json:"phone"`
	User
	NIS        *string `json:"nis"`
	NISN       *string `json:"nisn"`
	NIK        *string `json:"nik"`
	NUPTK      *string `json:"nuptk"`
	EmployeeID *string `json:"employee_id"`
}

func (r *Repository) detail(ctx context.Context, id uint64) (Detail, error) {
	var out Detail
	err := r.db.QueryRowContext(ctx, `SELECT u.id,u.login_id,COALESCE(u.email,''),u.full_name,u.avatar_url,u.bio,u.role,u.status,s.nis,s.nisn,COALESCE(t.nik,f.nik),COALESCE(t.nuptk,f.nuptk),COALESCE(t.employee_id,f.employee_id),u.birth_place,DATE_FORMAT(u.birth_date,'%Y-%m-%d'),u.phone FROM users u LEFT JOIN student_profiles s ON s.user_id=u.id LEFT JOIN teacher_profiles t ON t.user_id=u.id LEFT JOIN staff_profiles f ON f.user_id=u.id WHERE u.id=? AND u.deleted_at IS NULL`, id).Scan(&out.ID, &out.LoginID, &out.Email, &out.FullName, &out.AvatarURL, &out.Bio, &out.Role, &out.Status, &out.NIS, &out.NISN, &out.NIK, &out.NUPTK, &out.EmployeeID, &out.BirthPlace, &out.BirthDate, &out.Phone)
	return out, err
}

func updateUser(ctx context.Context, q database.Querier, id, actorID uint64, in CreateInput, hash string) error {
	var role string
	if err := q.QueryRowContext(ctx, "SELECT role FROM users WHERE id=? AND deleted_at IS NULL FOR UPDATE", id).Scan(&role); err != nil {
		return err
	}
	if role != in.Role {
		return inputError("Peran akun tidak dapat diubah melalui formulir ini.")
	}
	if id == actorID && in.Status != "active" {
		return inputError("Anda tidak dapat menonaktifkan akun sendiri.")
	}
	if _, err := q.ExecContext(ctx, "UPDATE users SET login_id=?,email=NULLIF(?,''),full_name=?,auth_version=auth_version+IF(status<>?,1,0),status=?,birth_place=?,birth_date=?,phone=? WHERE id=?", in.LoginID, in.Email, in.FullName, in.Status, in.Status, in.BirthPlace, in.BirthDate, in.Phone, id); err != nil {
		return err
	}
	if hash != "" {
		if _, err := q.ExecContext(ctx, "UPDATE users SET password_hash=?,auth_version=auth_version+1 WHERE id=?", hash, id); err != nil {
			return err
		}
	}
	var err error
	switch role {
	case "student":
		_, err = q.ExecContext(ctx, "UPDATE student_profiles SET nis=?,nisn=? WHERE user_id=?", in.NIS, in.NISN, id)
	case "teacher":
		_, err = q.ExecContext(ctx, "UPDATE teacher_profiles SET nik=?,nuptk=?,employee_id=? WHERE user_id=?", in.NIK, in.NUPTK, in.EmployeeID, id)
	default:
		_, err = q.ExecContext(ctx, "UPDATE staff_profiles SET employee_id=?,nik=?,nuptk=? WHERE user_id=?", in.EmployeeID, in.NIK, in.NUPTK, id)
	}
	if err != nil {
		return err
	}
	// Record only the fact that a password changed. Never place the password or
	// bcrypt hash in audit metadata.
	action := "user_updated"
	metadata := `{"password_changed":false}`
	if hash != "" {
		action = "user_password_changed"
		metadata = `{"password_changed":true}`
	}
	_, err = q.ExecContext(ctx, "INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,metadata_json) VALUES (?,?, 'users',?,?)", actorID, action, id, metadata)
	return err
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims.Role != "admin" {
		writeError(w, 403, "Akses khusus admin.")
		return
	}
	id, err := parseID(r)
	if err != nil || id == 0 {
		writeError(w, 400, "ID tidak valid.")
		return
	}
	var in CreateInput
	if !decodeManagement(w, r, &in) {
		return
	}
	if err = validateInput(&in, false); err != nil {
		managementError(w, err)
		return
	}
	hash := ""
	if in.Password != "" {
		b, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if e != nil {
			managementError(w, e)
			return
		}
		hash = string(b)
	}
	err = h.service.repository.transact(r.Context(), func(q database.Querier) error { return updateUser(r.Context(), q, id, claims.UserID, in, hash) })
	if err != nil {
		managementError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// Validate/hash before the transaction. Every row commits together, including profiles.
func (s *Service) Import(ctx context.Context, items []CreateInput) (int, error) {
	if len(items) == 0 || len(items) > 50 {
		return 0, inputError("Import menerima 1–50 pengguna per berkas.")
	}
	hashes := make([]string, len(items))
	for i := range items {
		if err := validateInput(&items[i], true); err != nil {
			return 0, inputError(fmt.Sprintf("Baris %d: %s", i+2, err))
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(items[i].Password), bcrypt.DefaultCost)
		if err != nil {
			return 0, err
		}
		hashes[i] = string(hash)
	}
	err := s.repository.transact(ctx, func(q database.Querier) error {
		for i, in := range items {
			if _, err := insertUser(ctx, q, in, hashes[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(items), nil
}
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Users []CreateInput `json:"users"`
	}
	if !decodeManagement(w, r, &in) {
		return
	}
	count, err := h.service.Import(r.Context(), in.Users)
	if err != nil {
		managementError(w, err)
		return
	}
	writeJSON(w, 201, map[string]int{"count": count})
}
