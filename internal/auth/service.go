package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"lms-website-be/internal/database"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	db        database.Querier
	jwtSecret []byte
	tokenTTL  time.Duration
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
	UserID  uint64 `json:"user_id"`
	LoginID string `json:"login_id"`
	Role    string `json:"role"`
	jwt.RegisteredClaims
}

var ErrCredentials = errors.New("login_id atau password salah, atau akun tidak aktif")
var ErrUnauthorized = errors.New("sesi tidak valid atau akun tidak aktif")

func NewService(db database.Querier, secret string) *Service {
	return &Service{db: db, jwtSecret: []byte(secret), tokenTTL: 8 * time.Hour}
}

func (s *Service) Login(ctx context.Context, loginID, password string) (LoginResult, error) {
	var user User
	var passwordHash string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, login_id, COALESCE(email,''), full_name, role, status, password_hash
		FROM users
		WHERE login_id = ? AND deleted_at IS NULL
	`, loginID).Scan(&user.ID, &user.LoginID, &user.Email, &user.FullName, &user.Role, &user.Status, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return LoginResult{}, ErrCredentials
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("mencari user: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return LoginResult{}, ErrCredentials
	}
	if user.Status != "active" || !validRole(user.Role) {
		return LoginResult{}, ErrCredentials
	}

	now := time.Now()
	claims := Claims{
		UserID:  user.ID,
		LoginID: user.LoginID,
		Role:    user.Role,
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
	err = s.db.QueryRowContext(ctx, "SELECT login_id, role, status FROM users WHERE id = ? AND deleted_at IS NULL", claims.UserID).Scan(&claims.LoginID, &claims.Role, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return Claims{}, ErrUnauthorized
	}
	if err != nil {
		return Claims{}, err
	}
	if status != "active" || !validRole(claims.Role) {
		return Claims{}, ErrUnauthorized
	}
	return claims, nil
}
