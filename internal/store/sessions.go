package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Session mirrors a row in the sessions table.
type Session struct {
	Token     string
	UserID    int64
	ExpiresAt time.Time
}

// CreateSession inserts a new session row binding a token to a user until
// expiresAt. The timestamp is stored as an RFC3339 UTC string so it sorts
// chronologically (lexicographic == chronological for that format) and
// compares cleanly both in SQL and after scanning back into Go.
func (s *Store) CreateSession(token string, userID int64, expiresAt time.Time) error {
	if _, err := s.db.Exec(
		`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, expiresAt.UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// DeleteSession removes the session with the given token. Deleting a token that
// does not exist is not an error (0 rows affected), which keeps logout idempotent.
func (s *Store) DeleteSession(token string) error {
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// SessionByToken looks up a session and its owning user in a single JOIN.
// Returns ErrNotFound when the token does not exist. Expiry is deliberately NOT
// checked here — the auth middleware decides what to do with an expired session.
func (s *Store) SessionByToken(token string) (Session, User, error) {
	row := s.db.QueryRow(
		`SELECT s.token, s.user_id, s.expires_at,
		        u.id, u.username, u.password_hash, u.role, u.created_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token = ?`,
		token,
	)
	var sess Session
	var u User
	err := row.Scan(
		&sess.Token, &sess.UserID, &sess.ExpiresAt,
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, User{}, ErrNotFound
	}
	if err != nil {
		return Session{}, User{}, fmt.Errorf("session by token: %w", err)
	}
	return sess, u, nil
}
