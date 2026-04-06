package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/thequ3st/napstarr/internal/database"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionNotFound    = errors.New("session not found")
)

// EnsureAdminUser creates an admin user if no users exist in the database.
// If password is empty, a random one is generated and printed to stdout.
func EnsureAdminUser(db *database.DB, username, password string) error {
	var count int
	if err := db.Reader.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	generated := false
	if password == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generate password: %w", err)
		}
		password = hex.EncodeToString(b)
		generated = true
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	id := database.NewID()
	_, err = db.Writer.Exec(
		`INSERT INTO users (id, username, display_name, password_hash, is_admin)
		 VALUES (?, ?, ?, ?, 1)`,
		id, username, username, string(hash),
	)
	if err != nil {
		return fmt.Errorf("insert admin: %w", err)
	}

	if generated {
		fmt.Printf("Admin user created: %s / %s\n", username, password)
	}

	return nil
}

// Login verifies credentials and creates a session token with 30-day expiry.
func Login(db *database.DB, username, password string) (string, error) {
	var user database.User
	err := db.Reader.QueryRow(
		"SELECT id, username, password_hash, is_admin FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin)

	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", fmt.Errorf("query user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	token, err := generateToken()
	if err != nil {
		return "", err
	}

	expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	_, err = db.Writer.Exec(
		"INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)",
		token, user.ID, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return token, nil
}

// Logout deletes a session by token.
func Logout(db *database.DB, token string) error {
	_, err := db.Writer.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

// ValidateSession checks that a token exists and has not expired, returning the associated user.
func ValidateSession(db *database.DB, token string) (*database.User, error) {
	var userID, expiresAt string
	err := db.Reader.QueryRow(
		"SELECT user_id, expires_at FROM sessions WHERE token = ?", token,
	).Scan(&userID, &expiresAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}

	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expiry: %w", err)
	}
	if time.Now().After(expiry) {
		// Clean up expired session.
		db.Writer.Exec("DELETE FROM sessions WHERE token = ?", token)
		return nil, ErrSessionExpired
	}

	var user database.User
	err = db.Reader.QueryRow(
		`SELECT id, instance_id, username, display_name, is_admin, created_at
		 FROM users WHERE id = ?`, userID,
	).Scan(&user.ID, &user.InstanceID, &user.Username, &user.DisplayName, &user.IsAdmin, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}

	return &user, nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
