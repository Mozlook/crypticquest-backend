package handlers

import (
	"database/sql"
	"encoding/json"
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

// authAdmin bootstraps an admin and a live session, returning its cookie.
func (e *testEnv) authAdmin(t *testing.T, username string) *http.Cookie {
	t.Helper()
	if _, err := e.st.EnsureAdmin(username, "hash"); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}
	u, err := e.st.UserByUsername(username)
	if err != nil {
		t.Fatalf("lookup admin: %v", err)
	}
	token := "token-" + username
	if err := e.st.CreateSession(token, u.ID, time.Now().Add(time.Hour)); err != nil {
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

// mustJSON decodes the recorder body into v, failing the test on error.
func mustJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
}

func TestAdminLevelsGating(t *testing.T) {
	e := newTestEnv(t)
	player := e.authUser(t, "alice")

	// No session -> 401; player session -> 403; on every admin verb.
	for _, ep := range []struct{ method, target string }{
		{http.MethodGet, "/api/admin/levels"},
		{http.MethodPost, "/api/admin/levels"},
		{http.MethodPut, "/api/admin/levels/1"},
		{http.MethodDelete, "/api/admin/levels/1"},
	} {
		if w := e.do(t, ep.method, ep.target, "", nil); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s no-auth: want 401, got %d", ep.method, ep.target, w.Code)
		}
		if w := e.do(t, ep.method, ep.target, "", player); w.Code != http.StatusForbidden {
			t.Fatalf("%s %s player: want 403, got %d", ep.method, ep.target, w.Code)
		}
	}
}

func TestAdminLevelsCRUD(t *testing.T) {
	e := newTestEnv(t)
	admin := e.authAdmin(t, "boss")

	// Create: admin response includes the flag.
	body := `{"order_index":10,"title":"Caesar","description":"narrative","flag":"flag{a}"}`
	w := e.do(t, http.MethodPost, "/api/admin/levels", body, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d body %s", w.Code, w.Body.String())
	}
	var created store.AdminLevel
	mustJSON(t, w, &created)
	if created.ID == 0 || created.Flag != "flag{a}" {
		t.Fatalf("created admin level missing id/flag: %+v", created)
	}
	sid := strconv.FormatInt(created.ID, 10)

	// Duplicate order_index -> 409.
	if w := e.do(t, http.MethodPost, "/api/admin/levels", body, admin); w.Code != http.StatusConflict {
		t.Fatalf("dup order_index: want 409, got %d", w.Code)
	}

	// Bad unlocks_tool_id -> 400.
	badRef := `{"order_index":20,"title":"x","description":"d","flag":"f","unlocks_tool_id":9999}`
	if w := e.do(t, http.MethodPost, "/api/admin/levels", badRef, admin); w.Code != http.StatusBadRequest {
		t.Fatalf("bad tool ref: want 400, got %d", w.Code)
	}

	// Missing required field -> 400.
	if w := e.do(t, http.MethodPost, "/api/admin/levels", `{"order_index":30,"title":"","description":"d","flag":"f"}`, admin); w.Code != http.StatusBadRequest {
		t.Fatalf("empty title: want 400, got %d", w.Code)
	}

	// List includes the flag.
	w = e.do(t, http.MethodGet, "/api/admin/levels", "", admin)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "flag{a}") {
		t.Fatalf("list should include flag: %d %s", w.Code, w.Body.String())
	}

	// Update.
	upd := `{"order_index":10,"title":"Caesar v2","description":"d2","flag":"flag{b}"}`
	w = e.do(t, http.MethodPut, "/api/admin/levels/"+sid, upd, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d %s", w.Code, w.Body.String())
	}
	var updated store.AdminLevel
	mustJSON(t, w, &updated)
	if updated.Title != "Caesar v2" || updated.Flag != "flag{b}" {
		t.Fatalf("update not applied: %+v", updated)
	}

	// Update missing -> 404.
	if w := e.do(t, http.MethodPut, "/api/admin/levels/9999", upd, admin); w.Code != http.StatusNotFound {
		t.Fatalf("update missing: want 404, got %d", w.Code)
	}

	// Delete -> 204, then 404.
	if w := e.do(t, http.MethodDelete, "/api/admin/levels/"+sid, "", admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d", w.Code)
	}
	if w := e.do(t, http.MethodDelete, "/api/admin/levels/"+sid, "", admin); w.Code != http.StatusNotFound {
		t.Fatalf("delete again: want 404, got %d", w.Code)
	}
}

func TestAdminHints(t *testing.T) {
	e := newTestEnv(t)
	admin := e.authAdmin(t, "boss")
	id := e.insertLevel(t, 10, "Caesar", "flag{a}")
	sid := strconv.FormatInt(id, 10)
	base := "/api/admin/levels/" + sid + "/hints"

	// Gating: player denied.
	player := e.authUser(t, "alice")
	if w := e.do(t, http.MethodPut, base, `{"hints":["x"]}`, player); w.Code != http.StatusForbidden {
		t.Fatalf("player PUT hints: want 403, got %d", w.Code)
	}

	// Initially empty.
	if w := e.do(t, http.MethodGet, base, "", admin); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("initial GET: want [], got %d %s", w.Code, w.Body.String())
	}

	// Replace with two, ordered.
	w := e.do(t, http.MethodPut, base, `{"hints":["first","second"]}`, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: want 200, got %d %s", w.Code, w.Body.String())
	}
	var hints []store.Hint
	mustJSON(t, w, &hints)
	if len(hints) != 2 || hints[0].Text != "first" || hints[1].Text != "second" {
		t.Fatalf("hints not in order: %+v", hints)
	}

	// Reorder/shrink to one.
	w = e.do(t, http.MethodPut, base, `{"hints":["only"]}`, admin)
	mustJSON(t, w, &hints)
	if len(hints) != 1 || hints[0].Text != "only" {
		t.Fatalf("after shrink: %+v", hints)
	}

	// Empty clears all.
	w = e.do(t, http.MethodPut, base, `{"hints":[]}`, admin)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("clear: want [], got %s", w.Body.String())
	}

	// Empty hint text -> 400.
	if w := e.do(t, http.MethodPut, base, `{"hints":["ok","  "]}`, admin); w.Code != http.StatusBadRequest {
		t.Fatalf("empty hint text: want 400, got %d", w.Code)
	}

	// Missing level -> 404 on both GET and PUT.
	if w := e.do(t, http.MethodGet, "/api/admin/levels/9999/hints", "", admin); w.Code != http.StatusNotFound {
		t.Fatalf("GET missing: want 404, got %d", w.Code)
	}
	if w := e.do(t, http.MethodPut, "/api/admin/levels/9999/hints", `{"hints":["x"]}`, admin); w.Code != http.StatusNotFound {
		t.Fatalf("PUT missing: want 404, got %d", w.Code)
	}
}

func TestAdminToolsCRUD(t *testing.T) {
	e := newTestEnv(t)
	admin := e.authAdmin(t, "boss")

	// Gating.
	player := e.authUser(t, "alice")
	if w := e.do(t, http.MethodGet, "/api/admin/tools", "", player); w.Code != http.StatusForbidden {
		t.Fatalf("player list tools: want 403, got %d", w.Code)
	}

	// Create.
	w := e.do(t, http.MethodPost, "/api/admin/tools", `{"type":"link","title":"CyberChef","description":"swiss army","content":"http://x"}`, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d %s", w.Code, w.Body.String())
	}
	var tool store.Tool
	mustJSON(t, w, &tool)
	if tool.ID == 0 || tool.Type != "link" {
		t.Fatalf("created tool: %+v", tool)
	}
	sid := strconv.FormatInt(tool.ID, 10)

	// Invalid type -> 400.
	if w := e.do(t, http.MethodPost, "/api/admin/tools", `{"type":"bogus","title":"x","content":"y"}`, admin); w.Code != http.StatusBadRequest {
		t.Fatalf("bad type: want 400, got %d", w.Code)
	}
	// Missing content -> 400.
	if w := e.do(t, http.MethodPost, "/api/admin/tools", `{"type":"pdf","title":"x","content":""}`, admin); w.Code != http.StatusBadRequest {
		t.Fatalf("empty content: want 400, got %d", w.Code)
	}

	// List.
	if w := e.do(t, http.MethodGet, "/api/admin/tools", "", admin); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "CyberChef") {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}

	// Update.
	w = e.do(t, http.MethodPut, "/api/admin/tools/"+sid, `{"type":"pdf","title":"Guide","description":"","content":"guide.pdf"}`, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d %s", w.Code, w.Body.String())
	}
	mustJSON(t, w, &tool)
	if tool.Type != "pdf" || tool.Content != "guide.pdf" {
		t.Fatalf("update not applied: %+v", tool)
	}
	if w := e.do(t, http.MethodPut, "/api/admin/tools/9999", `{"type":"link","title":"x","content":"y"}`, admin); w.Code != http.StatusNotFound {
		t.Fatalf("update missing: want 404, got %d", w.Code)
	}

	// A level unlocking the tool blocks deletion -> 409.
	if _, err := e.db.Exec(`INSERT INTO levels (order_index, title, description, flag, unlocks_tool_id) VALUES (10, 'L', 'd', 'f', ?)`, tool.ID); err != nil {
		t.Fatalf("insert referencing level: %v", err)
	}
	if w := e.do(t, http.MethodDelete, "/api/admin/tools/"+sid, "", admin); w.Code != http.StatusConflict {
		t.Fatalf("delete referenced: want 409, got %d", w.Code)
	}
	// Detach, then delete -> 204, then 404.
	if _, err := e.db.Exec(`UPDATE levels SET unlocks_tool_id = NULL`); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if w := e.do(t, http.MethodDelete, "/api/admin/tools/"+sid, "", admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d", w.Code)
	}
	if w := e.do(t, http.MethodDelete, "/api/admin/tools/"+sid, "", admin); w.Code != http.StatusNotFound {
		t.Fatalf("delete again: want 404, got %d", w.Code)
	}
}

func TestAdminUsers(t *testing.T) {
	e := newTestEnv(t)
	admin := e.authAdmin(t, "boss")
	bossID := mustUserID(t, e, "boss")

	// Four levels so progress can move.
	for _, oi := range []int{10, 20, 30, 40} {
		e.insertLevel(t, oi, "L", "f")
	}
	// A player to manage.
	player := e.authUser(t, "alice")
	aliceID := mustUserID(t, e, "alice")
	said := strconv.FormatInt(aliceID, 10)

	// Gating.
	if w := e.do(t, http.MethodGet, "/api/admin/users", "", player); w.Code != http.StatusForbidden {
		t.Fatalf("player list users: want 403, got %d", w.Code)
	}

	// List shows both, with current level.
	w := e.do(t, http.MethodGet, "/api/admin/users", "", admin)
	var users []store.AdminUser
	mustJSON(t, w, &users)
	if len(users) != 2 {
		t.Fatalf("want 2 users, got %+v", users)
	}

	// Set alice to level 3.
	w = e.do(t, http.MethodPut, "/api/admin/users/"+said, `{"level":3}`, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("set level: %d %s", w.Code, w.Body.String())
	}
	var au store.AdminUser
	mustJSON(t, w, &au)
	if au.CurrentLevel != 3 {
		t.Fatalf("want current level 3, got %+v", au)
	}

	// Promote alice to admin.
	w = e.do(t, http.MethodPut, "/api/admin/users/"+said, `{"role":"admin"}`, admin)
	mustJSON(t, w, &au)
	if au.Role != "admin" {
		t.Fatalf("want role admin, got %+v", au)
	}

	// Bad role / bad level -> 400.
	if w := e.do(t, http.MethodPut, "/api/admin/users/"+said, `{"role":"superuser"}`, admin); w.Code != http.StatusBadRequest {
		t.Fatalf("bad role: want 400, got %d", w.Code)
	}
	if w := e.do(t, http.MethodPut, "/api/admin/users/"+said, `{"level":0}`, admin); w.Code != http.StatusBadRequest {
		t.Fatalf("bad level: want 400, got %d", w.Code)
	}

	// Self-demote guard.
	if w := e.do(t, http.MethodPut, "/api/admin/users/"+strconv.FormatInt(bossID, 10), `{"role":"player"}`, admin); w.Code != http.StatusConflict {
		t.Fatalf("self-demote: want 409, got %d", w.Code)
	}

	// Reset password -> 200 with a non-empty plaintext.
	w = e.do(t, http.MethodPost, "/api/admin/users/"+said+"/reset-password", "", admin)
	if w.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}
	var pw struct {
		Password string `json:"password"`
	}
	mustJSON(t, w, &pw)
	if len(pw.Password) < 12 {
		t.Fatalf("temp password too short: %q", pw.Password)
	}
	// The reset must invalidate alice's existing session.
	if w := e.do(t, http.MethodGet, "/api/me", "", player); w.Code != http.StatusUnauthorized {
		t.Fatalf("session should be killed by reset: want 401, got %d", w.Code)
	}

	// Missing-user variants -> 404.
	if w := e.do(t, http.MethodPut, "/api/admin/users/9999", `{"level":2}`, admin); w.Code != http.StatusNotFound {
		t.Fatalf("update missing: want 404, got %d", w.Code)
	}
	if w := e.do(t, http.MethodPost, "/api/admin/users/9999/reset-password", "", admin); w.Code != http.StatusNotFound {
		t.Fatalf("reset missing: want 404, got %d", w.Code)
	}

	// Self-delete guard, then delete alice -> 204, then 404.
	if w := e.do(t, http.MethodDelete, "/api/admin/users/"+strconv.FormatInt(bossID, 10), "", admin); w.Code != http.StatusConflict {
		t.Fatalf("self-delete: want 409, got %d", w.Code)
	}
	if w := e.do(t, http.MethodDelete, "/api/admin/users/"+said, "", admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete alice: want 204, got %d", w.Code)
	}
	if w := e.do(t, http.MethodDelete, "/api/admin/users/"+said, "", admin); w.Code != http.StatusNotFound {
		t.Fatalf("delete again: want 404, got %d", w.Code)
	}
}

func mustUserID(t *testing.T, e *testEnv, username string) int64 {
	t.Helper()
	u, err := e.st.UserByUsername(username)
	if err != nil {
		t.Fatalf("lookup %s: %v", username, err)
	}
	return u.ID
}
