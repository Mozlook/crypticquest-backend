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

func TestRequireLoginSlidingExpiration(t *testing.T) {
	st := newTestStore(t)
	requireLogin := RequireLogin(st, auth.NewSessionCookie("", false, "Lax"))
	uid, err := st.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	pass := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	hit := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		requireLogin(pass).ServeHTTP(rec, req)
		return rec
	}

	t.Run("near expiry is extended and cookie refreshed", func(t *testing.T) {
		near := time.Now().Add(10 * 24 * time.Hour) // inside the 15-day threshold
		if err := st.CreateSession("near", uid, near); err != nil {
			t.Fatalf("create session: %v", err)
		}
		rec := hit("near")
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		sess, _, err := st.SessionByToken("near")
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if !sess.ExpiresAt.After(near.Add(time.Hour)) {
			t.Fatalf("expiry not extended: got %v, was %v", sess.ExpiresAt, near)
		}
		if len(rec.Result().Cookies()) == 0 {
			t.Fatal("expected a refreshed session cookie")
		}
	})

	t.Run("far expiry is left untouched", func(t *testing.T) {
		far := time.Now().Add(25 * 24 * time.Hour).Truncate(time.Second) // beyond threshold
		if err := st.CreateSession("far", uid, far); err != nil {
			t.Fatalf("create session: %v", err)
		}
		rec := hit("far")
		sess, _, err := st.SessionByToken("far")
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if !sess.ExpiresAt.Equal(far) {
			t.Fatalf("expiry should be unchanged: got %v want %v", sess.ExpiresAt, far)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Fatal("did not expect a refreshed cookie for a far-future session")
		}
	})
}

func TestRequireAdmin(t *testing.T) {
	st := newTestStore(t)
	requireAdmin := RequireAdmin(st, auth.NewSessionCookie("", false, "Lax"))

	// A regular player and a bootstrapped admin, each with a live session.
	playerID, err := st.CreateUser("player", "hash")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}
	if _, err := st.EnsureAdmin("boss", "hash"); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}
	adminUser, err := st.UserByUsername("boss")
	if err != nil {
		t.Fatalf("lookup admin: %v", err)
	}
	future := time.Now().Add(time.Hour)
	if err := st.CreateSession("player-tok", playerID, future); err != nil {
		t.Fatalf("player session: %v", err)
	}
	if err := st.CreateSession("admin-tok", adminUser.ID, future); err != nil {
		t.Fatalf("admin session: %v", err)
	}

	var ran bool
	guarded := requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	}))

	hit := func(token string) *httptest.ResponseRecorder {
		ran = false
		r := httptest.NewRequest(http.MethodGet, "/api/admin/x", nil)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
		}
		rec := httptest.NewRecorder()
		guarded.ServeHTTP(rec, r)
		return rec
	}

	t.Run("no session -> 401", func(t *testing.T) {
		if rec := hit(""); rec.Code != http.StatusUnauthorized || ran {
			t.Fatalf("want 401 no-run, got %d ran=%v", rec.Code, ran)
		}
	})
	t.Run("player -> 403", func(t *testing.T) {
		if rec := hit("player-tok"); rec.Code != http.StatusForbidden || ran {
			t.Fatalf("want 403 no-run, got %d ran=%v", rec.Code, ran)
		}
	})
	t.Run("admin -> 200", func(t *testing.T) {
		if rec := hit("admin-tok"); rec.Code != http.StatusOK || !ran {
			t.Fatalf("want 200 run, got %d ran=%v", rec.Code, ran)
		}
	})
}
