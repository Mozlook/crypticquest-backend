package store

import "fmt"

// EnsureAdmin bootstraps the first admin account. It is idempotent and safe to
// call on every startup: if any admin already exists it does nothing. Only when
// the table has no admin does it create one with the given (already-hashed)
// password. Returns whether an account was created.
func (s *Store) EnsureAdmin(username, passwordHash string) (created bool, err error) {
	var adminCount int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM users WHERE role = 'admin'`,
	).Scan(&adminCount); err != nil {
		return false, fmt.Errorf("count admins: %w", err)
	}
	if adminCount > 0 {
		return false, nil // already bootstrapped
	}

	if _, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, role) VALUES (?, ?, 'admin')`,
		username, passwordHash,
	); err != nil {
		return false, fmt.Errorf("create admin %q (is the username already taken?): %w", username, err)
	}
	return true, nil
}
