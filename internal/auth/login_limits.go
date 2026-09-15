package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var ErrLoginLimited = errors.New("Terlalu banyak percobaan masuk. Tunggu 1 menit lalu coba kembali.")

type loginWindow struct {
	attempts int
	expires  time.Time
}
type loginLimiter struct {
	mu          sync.Mutex
	entries     map[[32]byte]loginWindow
	nextCleanup time.Time
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{entries: make(map[[32]byte]loginWindow)} }
func (l *loginLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !now.Before(l.nextCleanup) {
		for k, v := range l.entries {
			if !now.Before(v.expires) {
				delete(l.entries, k)
			}
		}
		l.nextCleanup = now.Add(time.Minute)
	}
	k := sha256.Sum256([]byte(key))
	v, exists := l.entries[k]
	if !exists || !now.Before(v.expires) {
		// Bounded memory; do not evict live limits when random identities flood login.
		if !exists && len(l.entries) >= 10000 {
			return false
		}
		v = loginWindow{expires: now.Add(time.Minute)}
	}
	if v.attempts >= 10 {
		return false
	}
	v.attempts++
	l.entries[k] = v
	return true
}

var dummyPasswordHash = sync.OnceValue(func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("unused-login-timing-value"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return hash
})

func (s *Service) comparePassword(ctx context.Context, hash []byte, password string) error {
	select {
	case s.passwordSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.passwordSlots }()
	if err := ctx.Err(); err != nil {
		return err
	}
	err := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return ErrCredentials
	}
	return nil
}
