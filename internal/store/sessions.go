package store

import (
	"fmt"
	"time"
)

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
