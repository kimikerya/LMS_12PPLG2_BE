package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseTokenRejectsWrongSecret(t *testing.T) {
	service := NewService(nil, "correct-secret")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: 1,
		Role:   "student",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	signed, err := token.SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ParseToken(signed); err == nil {
		t.Fatal("token dengan secret yang salah seharusnya ditolak")
	}
}

func TestParseTokenValidation(t *testing.T) {
	secret := "test-secret"
	s := NewService(nil, secret)
	for _, tc := range []struct {
		name          string
		expiry        *jwt.NumericDate
		method        jwt.SigningMethod
		subject, role string
		ok            bool
	}{
		{"valid", jwt.NewNumericDate(time.Now().Add(time.Hour)), jwt.SigningMethodHS256, "1", "student", true},
		{"expired", jwt.NewNumericDate(time.Now().Add(-time.Hour)), jwt.SigningMethodHS256, "1", "student", false},
		{"missing expiration", nil, jwt.SigningMethodHS256, "1", "student", false},
		{"wrong algorithm", jwt.NewNumericDate(time.Now().Add(time.Hour)), jwt.SigningMethodHS384, "1", "student", false},
		{"mismatched identity", jwt.NewNumericDate(time.Now().Add(time.Hour)), jwt.SigningMethodHS256, "2", "student", false},
		{"unknown role", jwt.NewNumericDate(time.Now().Add(time.Hour)), jwt.SigningMethodHS256, "1", "owner", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := jwt.NewWithClaims(tc.method, Claims{UserID: 1, Role: tc.role, RegisteredClaims: jwt.RegisteredClaims{Subject: tc.subject, ExpiresAt: tc.expiry}})
			raw, err := token.SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.ParseToken(raw)
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected validation: %v", err)
			}
		})
	}
}
