package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// rowScanner is satisfied by both *sql.Row (QueryRow) and *sql.Rows (Query),
// so a single scan helper serves single-row and multi-row reads alike.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanUser maps one row of the standard user column order into a User.
func scanUser(row rowScanner) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
	return u, err
}

// User roles. SQLite has no enum, so roles are plain strings constrained by
// convention and validated in code.
const (
	RolePlayer = "player"
	RoleAdmin  = "admin"
)

// User mirrors a row in the users table. PasswordHash is the bcrypt output and
// must never be serialized to clients.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

// CreateUser inserts a new player account (role defaults to 'player' in the
// schema) and returns its id. If the username is already taken it returns
// ErrUsernameTaken, relying on the UNIQUE constraint so the check and insert
// are atomic (no race between a separate "does it exist?" query and the insert).
func (s *Store) CreateUser(username, passwordHash string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		username, passwordHash,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrUsernameTaken
		}
		return 0, fmt.Errorf("create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create user (last insert id): %w", err)
	}
	return id, nil
}

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

// UserByUsername looks up a user by username. Returns ErrNotFound when no such
// user exists, so callers can distinguish "wrong username" from a real DB error.
func (s *Store) UserByUsername(username string) (User, error) {
	row := s.db.QueryRow(
		`SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?`,
		username,
	)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("user by username: %w", err)
	}
	return u, nil
}
