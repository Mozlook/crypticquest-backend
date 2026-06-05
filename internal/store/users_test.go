package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"crypticquest"
	"crypticquest/internal/db"
)

// newTestStore opens a fresh migrated SQLite database in a temp dir and returns
// a Store backed by it. Shared by store tests.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, crypticquest.MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func TestCreateUser(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if id <= 0 {
		t.Fatalf("want positive id, got %d", id)
	}

	if _, err := s.CreateUser("alice", "hash2"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate username: want ErrUsernameTaken, got %v", err)
	}
}

func TestUserByUsername(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	u, err := s.UserByUsername("alice")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if u.ID != id || u.Username != "alice" || u.PasswordHash != "hash1" {
		t.Fatalf("unexpected user: %+v", u)
	}
	if u.Role != RolePlayer {
		t.Fatalf("want default role %q, got %q", RolePlayer, u.Role)
	}
	if u.CreatedAt.IsZero() {
		t.Fatalf("created_at did not scan into time.Time (zero value)")
	}

	if _, err := s.UserByUsername("nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user: want ErrNotFound, got %v", err)
	}
}

func TestAdminUserManagement(t *testing.T) {
	s := newTestStore(t)

	// Four levels to move progress through.
	for _, oi := range []int{10, 20, 30, 40} {
		if _, err := s.db.Exec(`INSERT INTO levels (order_index, title, description, flag) VALUES (?, 'L', 'd', 'f')`, oi); err != nil {
			t.Fatalf("insert level: %v", err)
		}
	}
	uid, err := s.CreateUser("alice", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Fresh account lists at current level 1.
	users, err := s.ListUsers()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(users) != 1 || users[0].CurrentLevel != 1 || users[0].Role != RolePlayer {
		t.Fatalf("fresh list: %+v", users)
	}

	// Set progress to level 3 -> solved first 2 -> current level 3.
	if err := s.SetUserProgressToLevel(uid, 3); err != nil {
		t.Fatalf("set progress: %v", err)
	}
	if cl, _ := s.CurrentLevel(uid); cl != 3 {
		t.Fatalf("want current level 3, got %d", cl)
	}

	// Lowering to level 1 clears all progress (idempotent rewrite).
	if err := s.SetUserProgressToLevel(uid, 1); err != nil {
		t.Fatalf("reset progress: %v", err)
	}
	if cl, _ := s.CurrentLevel(uid); cl != 1 {
		t.Fatalf("want current level 1, got %d", cl)
	}

	// Promote to admin.
	if err := s.SetUserRole(uid, RoleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	if u, _ := s.UserByUsername("alice"); u.Role != RoleAdmin {
		t.Fatalf("role not updated: %+v", u)
	}

	// Password hash update.
	if err := s.UpdatePasswordHash(uid, "newhash"); err != nil {
		t.Fatalf("update hash: %v", err)
	}
	if u, _ := s.UserByUsername("alice"); u.PasswordHash != "newhash" {
		t.Fatalf("hash not updated")
	}

	// Missing-user variants -> ErrNotFound.
	if err := s.SetUserRole(9999, RoleAdmin); !errors.Is(err, ErrNotFound) {
		t.Fatalf("role missing: want ErrNotFound, got %v", err)
	}
	if err := s.SetUserProgressToLevel(9999, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("progress missing: want ErrNotFound, got %v", err)
	}
	if err := s.UpdatePasswordHash(9999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hash missing: want ErrNotFound, got %v", err)
	}

	// Delete cascades sessions + progress; second delete is 404.
	if err := s.CreateSession("tok", uid, timeNowPlusHour()); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.DeleteUser(uid); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var sessCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, uid).Scan(&sessCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessCount != 0 {
		t.Fatalf("sessions should cascade, got %d", sessCount)
	}
	if err := s.DeleteUser(uid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete again: want ErrNotFound, got %v", err)
	}
}

func timeNowPlusHour() time.Time { return time.Now().Add(time.Hour) }
