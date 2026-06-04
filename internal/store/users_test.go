package store

import (
	"errors"
	"path/filepath"
	"testing"

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
