package store

import (
	"errors"

	"modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

// Sentinel errors the store returns so handlers can react without inspecting
// driver-specific error text. Compare with errors.Is.
var (
	// ErrUsernameTaken is returned when a username already exists.
	ErrUsernameTaken = errors.New("username already taken")
	// ErrNotFound is returned when a looked-up row does not exist.
	ErrNotFound = errors.New("not found")
)

// isUniqueViolation reports whether err is a SQLite UNIQUE-constraint failure.
// Detected via the driver's typed error code (robust against message wording).
func isUniqueViolation(err error) bool {
	var sqErr *sqlite.Error
	return errors.As(err, &sqErr) && sqErr.Code() == sqlitelib.SQLITE_CONSTRAINT_UNIQUE
}
