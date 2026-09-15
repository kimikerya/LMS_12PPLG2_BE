package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"lms-website-be/internal/database"

	"github.com/golang-jwt/jwt/v5"
)

type Service struct {
	db            database.Querier
	jwtSecret     []byte
	tokenTTL      time.Duration
	limiter       *loginLimiter
	passwordSlots chan struct{}
}

type User struct {
	ID       uint64 `json:"id"`
	LoginID  string `json:"login_id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

type LoginResult struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type Claims struct {
	AuthVersion uint64 `json:"auth_version"`
	UserID      uint64 `json:"user_id"`
	LoginID     string `json:"login_id"`
	Role        string `json:"role"`
	jwt.RegisteredClaims
}

var ErrCredentials = errors.New("NIS/NIP atau password salah, identitas tidak unik, atau akun tidak aktif")
var ErrUnauthorized = errors.New("sesi tidak valid atau akun tidak aktif")

func NewService(db database.Querier, secret string) *Service {
	return &Service{db: db, jwtSecret: []byte(secret), tokenTTL: 8 * time.Hour, limiter: newLoginLimiter(), passwordSlots: make(chan struct{}, 4)}
}

func (s *Service) Login(ctx context.Context, loginID, password string) (LoginResult, error) {
	loginID = strings.TrimSpace(loginID)
	if loginID == "" || utf8.RuneCountInString(loginID) > 100 || len(password) == 0 || len(password) > 72 {
		return LoginResult{}, ErrCredentials
	}
	if !s.limiter.allow("identifier:"+strings.ToLower(loginID), time.Now()) {
		return LoginResult{}, ErrLoginLimited
	}
	var user User
	var passwordHash string
	var authVersion uint64
	// Only school identifiers may authenticate. login_id remains internal metadata.
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.login_id, COALESCE(u.email,''), u.full_name, u.role, u.status, u.password_hash,u.auth_version
		FROM (
		 SELECT u.id FROM student_profiles p JOIN users u ON u.id=p.user_id WHERE p.nis=? AND u.role='student' AND u.deleted_at IS NULL
		 UNION ALL
		 SELECT u.id FROM teacher_profiles p JOIN users u ON u.id=p.user_id WHERE p.employee_id=? AND u.role='teacher' AND u.deleted_at IS NULL
		 UNION ALL
		 SELECT u.id FROM staff_profiles p JOIN users u ON u.id=p.user_id WHERE p.employee_id=? AND u.role IN ('admin','curriculum','principal') AND u.deleted_at IS NULL
		) matched JOIN users u ON u.id=matched.id
		LIMIT 2
	`, loginID, loginID, loginID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("mencari user: %w", err)
	}
	if !rows.Next() {
		err = rows.Err()
		rows.Close()
		if err != nil {
			return LoginResult{}, err
		}
		if err = s.comparePassword(ctx, dummyPasswordHash(), password); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrCredentials
	}
	err = rows.Scan(&user.ID, &user.LoginID, &user.Email, &user.FullName, &user.Role, &user.Status, &passwordHash, &authVersion)
	ambiguous := rows.Next()
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return LoginResult{}, fmt.Errorf("mencari user: %w", err)
	}
	if ambiguous {
		if err = s.comparePassword(ctx, dummyPasswordHash(), password); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrCredentials
	}
	// Resolved IDs also cover aliases accepted by the database collation.
	if !s.limiter.allow("account:"+strconv.FormatUint(user.ID, 10), time.Now()) {
		return LoginResult{}, ErrLoginLimited
	}
	if err := s.comparePassword(ctx, []byte(passwordHash), password); err != nil {
		return LoginResult{}, err
	}
	if user.Status != "active" || !validRole(user.Role) {
		return LoginResult{}, ErrCredentials
	}

	now := time.Now()
	claims := Claims{
		AuthVersion: authVersion,
		UserID:      user.ID,
		LoginID:     user.LoginID,
		Role:        user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   fmt.Sprintf("%d", user.ID),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return LoginResult{}, fmt.Errorf("membuat token: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, "UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?", user.ID); err != nil {
		return LoginResult{}, fmt.Errorf("memperbarui waktu login: %w", err)
	}
	return LoginResult{Token: signed, User: user}, nil
}

func (s *Service) ParseToken(tokenString string) (Claims, error) {
	var claims Claims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("metode token tidak diizinkan")
		}
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || token == nil || !token.Valid || claims.UserID == 0 || claims.Subject != strconv.FormatUint(claims.UserID, 10) || !validRole(claims.Role) {
		return Claims{}, fmt.Errorf("token tidak valid")
	}
	return claims, nil
}

func validRole(role string) bool {
	switch role {
	case "student", "teacher", "admin", "curriculum", "principal":
		return true
	}
	return false
}

// Authenticate reloads role/status so disabling an account takes effect on the next request.
func (s *Service) Authenticate(ctx context.Context, raw string) (Claims, error) {
	claims, err := s.ParseToken(raw)
	if err != nil {
		return Claims{}, ErrUnauthorized
	}
	var status string
	var version uint64
	err = s.db.QueryRowContext(ctx, "SELECT login_id, role, status,auth_version FROM users WHERE id = ? AND deleted_at IS NULL", claims.UserID).Scan(&claims.LoginID, &claims.Role, &status, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return Claims{}, ErrUnauthorized
	}
	if err != nil {
		return Claims{}, err
	}
	if status != "active" || !validRole(claims.Role) || version != claims.AuthVersion {
		return Claims{}, ErrUnauthorized
	}
	return claims, nil
}
