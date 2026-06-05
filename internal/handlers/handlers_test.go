package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"crypticquest"
	"crypticquest/internal/auth"
	"crypticquest/internal/db"
	"crypticquest/internal/store"
)

// testEnv wires a real router over a fresh migrated database, so handler tests
// exercise the whole stack (router + RequireLogin middleware + store) the way a
// request actually flows. The raw *sql.DB is kept so tests can insert content
// directly — there is no player-flow store method to create levels (that is the
// admin panel's job, Phase 6).
type testEnv struct {
	router http.Handler
	db     *sql.DB
	st     *store.Store
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, crypticquest.MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := store.New(database)
	h := New(st, auth.NewSessionCookie("", false, "Lax"), t.TempDir())
	return &testEnv{router: h.Routes(), db: database, st: st}
}

// authUser creates a user plus a live session and returns the cookie carrying
// its token, ready to attach to a request.
func (e *testEnv) authUser(t *testing.T, username string) *http.Cookie {
	t.Helper()
	uid, err := e.st.CreateUser(username, "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token := "token-" + username
	if err := e.st.CreateSession(token, uid, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return &http.Cookie{Name: auth.SessionCookieName, Value: token}
}

// insertLevel adds a level and returns its id. order_index 10 means rank 1, so a
// fresh account (current level 1) can reach it.
func (e *testEnv) insertLevel(t *testing.T, orderIndex int, title, flag string) int64 {
	t.Helper()
	res, err := e.db.Exec(
		`INSERT INTO levels (order_index, title, description, flag) VALUES (?, ?, 'narrative', ?)`,
		orderIndex, title, flag,
	)
	if err != nil {
		t.Fatalf("insert level: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// do issues a request through the router with the given cookie and returns the
// recorder. body is sent as-is (empty string = no body).
func (e *testEnv) do(t *testing.T, method, target, body string, c *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, r)
	return w
}

// TestSubmitFlagValidation pins the comparison rule: case-insensitive, exact,
// with no trimming (the precise format is part of the challenge).
func TestSubmitFlagValidation(t *testing.T) {
	e := newTestEnv(t)
	id := e.insertLevel(t, 10, "Caesar", "FlagWithCase")
	cookie := e.authUser(t, "alice")
	target := "/api/levels/" + strconv.FormatInt(id, 10) + "/submit"

	cases := []struct {
		name, answer string
		want         bool
	}{
		{"exact", `FlagWithCase`, true},
		{"different case", `FLAGWITHCASE`, true},
		{"lowercased", `flagwithcase`, true},
		{"wrong", `nope`, false},
		{"trailing space not trimmed", `FlagWithCase `, false},
		{"leading space not trimmed", ` FlagWithCase`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := e.do(t, http.MethodPost, target, `{"answer":"`+tc.answer+`"}`, cookie)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
			}
			want := `{"correct":` + boolStr(tc.want) + `}`
			if got := strings.TrimSpace(w.Body.String()); got != want {
				t.Fatalf("answer %q: got %s, want %s", tc.answer, got, want)
			}
		})
	}
}

// TestFlagNeverInPlayerResponses guards the core secret: the flag must not leak
// through any player-facing endpoint, including the submit response.
func TestFlagNeverInPlayerResponses(t *testing.T) {
	e := newTestEnv(t)
	const flag = "SuperSecretFlag123"
	id := e.insertLevel(t, 10, "Caesar", flag)
	if _, err := e.db.Exec(
		`INSERT INTO hints (level_id, order_index, text) VALUES (?, 1, 'a hint')`, id,
	); err != nil {
		t.Fatalf("insert hint: %v", err)
	}
	cookie := e.authUser(t, "alice")
	sid := strconv.FormatInt(id, 10)

	endpoints := []struct {
		method, target, body string
	}{
		{http.MethodGet, "/api/levels", ""},
		{http.MethodGet, "/api/levels/" + sid, ""},
		{http.MethodGet, "/api/levels/" + sid + "/hints", ""},
		{http.MethodPost, "/api/levels/" + sid + "/submit", `{"answer":"wrong"}`},
		{http.MethodPost, "/api/levels/" + sid + "/submit", `{"answer":"` + flag + `"}`}, // correct, still must not echo it
	}
	for _, ep := range endpoints {
		w := e.do(t, ep.method, ep.target, ep.body, cookie)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s: status %d, body %s", ep.method, ep.target, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), flag) {
			t.Fatalf("%s %s leaked the flag: %s", ep.method, ep.target, w.Body.String())
		}
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
