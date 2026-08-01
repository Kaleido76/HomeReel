package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	settingsKeyHash = "auth.password_hash"
	defaultDays     = 30
	tokenBytes      = 32
)

var ErrInvalidPassword = errors.New("invalid password")

// Service owns the single-user access password and session lifecycle
// (ADR-002). The bcrypt hash lives in the settings table; sessions live in
// the sessions table. Session tokens are stored as SHA-256 digests.
type Service struct {
	db   *sql.DB
	days int
}

func New(database *sql.DB, sessionDays int) *Service {
	if sessionDays <= 0 {
		sessionDays = defaultDays
	}
	return &Service{db: database, days: sessionDays}
}

// SessionDays returns the configured session lifetime in days.
func (s *Service) SessionDays() int { return s.days }

// EnsurePassword makes sure a usable password hash exists in the DB.
//   - configured != "" → the configured value always wins; rehash and store.
//   - configured == "" → reuse the stored hash; if none exists generate a
//     random password, store it and return it so the caller can print it once.
func (s *Service) EnsurePassword(ctx context.Context, configured string) (generated string, err error) {
	if configured != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(configured), bcrypt.DefaultCost)
		if err != nil {
			return "", fmt.Errorf("hash password: %w", err)
		}
		if err := s.setHash(ctx, string(hash)); err != nil {
			return "", err
		}
		return "", nil
	}
	hash, err := s.getHash(ctx)
	if err == nil && hash != "" {
		return "", nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	generated, err = newToken()
	if err != nil {
		return "", err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(generated), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash generated password: %w", err)
	}
	if err := s.setHash(ctx, string(hashed)); err != nil {
		return "", err
	}
	return generated, nil
}

// Login verifies the password and issues a new session token.
func (s *Service) Login(ctx context.Context, password string) (string, error) {
	hash, err := s.getHash(ctx)
	if err != nil {
		return "", fmt.Errorf("load password hash: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", ErrInvalidPassword
	}
	token, err := newToken()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().AddDate(0, 0, s.days).UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, created_at, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), time.Now().UTC().Format(time.RFC3339), expiresAt); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

// Valid reports whether the token belongs to a non-expired session.
func (s *Service) Valid(ctx context.Context, token string) (bool, error) {
	var expiresAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT expires_at FROM sessions WHERE token = ?`, hashToken(token)).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false, err
	}
	return time.Now().UTC().Before(expires), nil
}

// Logout invalidates the session identified by token.
func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, hashToken(token))
	return err
}

func (s *Service) getHash(ctx context.Context) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, settingsKeyHash).Scan(&value)
	return value, err
}

func (s *Service) setHash(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		settingsKeyHash, hash, time.Now().UTC().Format(time.RFC3339))
	return err
}

func newToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
