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
	// ErrOrderIndexTaken is returned when a level's order_index collides with an
	// existing one (the UNIQUE constraint).
	ErrOrderIndexTaken = errors.New("order_index already taken")
	// ErrInvalidReference is returned when a write points a foreign key at a row
	// that does not exist (e.g. a tool's unlocks_at_level_id naming a missing level).
	ErrInvalidReference = errors.New("invalid reference")
)

// isUniqueViolation reports whether err is a SQLite UNIQUE-constraint failure.
// Detected via the driver's typed error code (robust against message wording).
func isUniqueViolation(err error) bool {
	var sqErr *sqlite.Error
	return errors.As(err, &sqErr) && sqErr.Code() == sqlitelib.SQLITE_CONSTRAINT_UNIQUE
}

// isForeignKeyViolation reports whether err is a SQLite FOREIGN KEY failure —
// raised both when a write references a missing row and when a delete is blocked
// by a still-existing referrer (foreign_keys is ON per connection).
func isForeignKeyViolation(err error) bool {
	var sqErr *sqlite.Error
	return errors.As(err, &sqErr) && sqErr.Code() == sqlitelib.SQLITE_CONSTRAINT_FOREIGNKEY
}
