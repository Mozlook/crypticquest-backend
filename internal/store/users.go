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

// --- Admin surface ----------------------------------------------------------

// AdminUser is the player-management view of an account. It omits the password
// hash and adds the derived CurrentLevel (solved count + 1, the same ordinal the
// player sees), so the admin list needs no extra per-row query.
type AdminUser struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	CurrentLevel int       `json:"current_level"`
}

// ListUsers returns every account with its current level, ordered by id. The
// LEFT JOIN + COUNT computes current level in the same query (no N+1).
func (s *Store) ListUsers() ([]AdminUser, error) {
	rows, err := s.db.Query(
		`SELECT u.id, u.username, u.role, u.created_at, COUNT(up.id) + 1
		 FROM users u
		 LEFT JOIN user_progress up ON up.user_id = u.id
		 GROUP BY u.id
		 ORDER BY u.id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := []AdminUser{} // non-nil so JSON encodes [] not null
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt, &u.CurrentLevel); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

// AdminUserByID returns one account with its current level, or ErrNotFound.
func (s *Store) AdminUserByID(id int64) (AdminUser, error) {
	var u AdminUser
	err := s.db.QueryRow(
		`SELECT u.id, u.username, u.role, u.created_at,
		        (SELECT COUNT(*) FROM user_progress WHERE user_id = u.id) + 1
		 FROM users u WHERE u.id = ?`,
		id,
	).Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt, &u.CurrentLevel)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminUser{}, ErrNotFound
	}
	if err != nil {
		return AdminUser{}, fmt.Errorf("admin user by id: %w", err)
	}
	return u, nil
}

// SetUserRole changes an account's role. The caller validates the role value.
// Returns ErrNotFound if no such user.
func (s *Store) SetUserRole(id int64, role string) error {
	res, err := s.db.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, id)
	if err != nil {
		return fmt.Errorf("set user role: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetUserProgressToLevel rewrites a user's progress so their current level
// becomes the given level (the simple panel mode). Current level = solved + 1,
// so reaching level N means the first N-1 levels (by order_index) are solved.
// Done in one transaction: clear all progress, then mark the first N-1. Returns
// ErrNotFound if no such user. The caller validates level >= 1.
func (s *Store) SetUserProgressToLevel(id int64, level int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("set progress: %w", err)
	}
	defer tx.Rollback() // no-op after a successful Commit

	var one int
	err = tx.QueryRow(`SELECT 1 FROM users WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("set progress: lookup user: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM user_progress WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("set progress: clear: %w", err)
	}
	// solved = level - 1; the first that many levels by order_index. LIMIT 0 (or
	// negative clamped to 0) inserts nothing, leaving the user on level 1.
	solved := level - 1
	if solved > 0 {
		if _, err := tx.Exec(
			`INSERT INTO user_progress (user_id, level_id)
			 SELECT ?, id FROM levels ORDER BY order_index LIMIT ?`,
			id, solved,
		); err != nil {
			return fmt.Errorf("set progress: insert: %w", err)
		}
	}
	return tx.Commit()
}

// UpdatePasswordHash overwrites an account's password hash (admin reset flow).
// Returns ErrNotFound if no such user.
func (s *Store) UpdatePasswordHash(id int64, passwordHash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	if err != nil {
		return fmt.Errorf("update password hash: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUser removes an account by id; sessions and progress cascade away.
// Returns ErrNotFound if there was nothing to delete.
func (s *Store) DeleteUser(id int64) error {
	res, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
