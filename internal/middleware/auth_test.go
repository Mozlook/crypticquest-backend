package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"crypticquest"
	"crypticquest/internal/auth"
	"crypticquest/internal/db"
	"crypticquest/internal/store"
)

// newTestStore opens a fresh migrated SQLite database in a temp dir.
func newTestStore(t *testing.T) *store.Store {
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
	return store.New(database)
}

func TestRequireLogin(t *testing.T) {
	st := newTestStore(t)
	requireLogin := RequireLogin(st, auth.NewSessionCookie("", false, "Lax"))

	uid, err := st.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// guarded wraps a sentinel handler that records whether it ran and the
	// username it saw in context.
	guarded := func(ran *bool, name *string) http.Handler {
		return requireLogin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u, ok := UserFromContext(r.Context()); ok {
				*ran = true
				*name = u.Username
			}
			w.WriteHeader(http.StatusOK)
		}))
	}

	t.Run("no cookie -> 401", func(t *testing.T) {
		var ran bool
		var name string
		rec := httptest.NewRecorder()
		guarded(&ran, &name).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != http.StatusUnauthorized || ran {
			t.Fatalf("want 401 and no run, got %d ran=%v", rec.Code, ran)
		}
	})

	t.Run("unknown token -> 401", func(t *testing.T) {
		var ran bool
		var name string
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "nope"})
		rec := httptest.NewRecorder()
		guarded(&ran, &name).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || ran {
			t.Fatalf("want 401 and no run, got %d ran=%v", rec.Code, ran)
		}
	})

	t.Run("expired session -> 401 and deleted", func(t *testing.T) {
		if err := st.CreateSession("expired", uid, time.Now().Add(-time.Hour)); err != nil {
			t.Fatalf("create session: %v", err)
		}
		var ran bool
		var name string
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "expired"})
		rec := httptest.NewRecorder()
		guarded(&ran, &name).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || ran {
			t.Fatalf("want 401 and no run, got %d ran=%v", rec.Code, ran)
		}
		if _, _, err := st.SessionByToken("expired"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expired session should be deleted (lazy cleanup), got %v", err)
		}
	})

	t.Run("valid session -> 200 with user in context", func(t *testing.T) {
		if err := st.CreateSession("valid", uid, time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("create session: %v", err)
		}
		var ran bool
		var name string
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "valid"})
		rec := httptest.NewRecorder()
		guarded(&ran, &name).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !ran {
			t.Fatalf("want 200 and run, got %d ran=%v", rec.Code, ran)
		}
		if name != "alice" {
			t.Fatalf("handler saw wrong user in context: %q", name)
		}
	})
}
