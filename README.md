# crypticquest-backend

The HTTP/JSON API behind **CrypticQuest** — a cryptography puzzle game where
players solve sequential levels, submit flags, and unlock tools.

Built deliberately small: **Go 1.26 + `net/http`**, **SQLite** via
`modernc.org/sqlite` (pure Go, no cgo), `golang-migrate`, and `bcrypt`. No web
framework, no ORM — just the standard library and hand-written SQL.

---

## Stack & principles

- **Routing**: the Go 1.22 `net/http.ServeMux` (method + path patterns, path
  values). No third-party router.
- **Persistence**: a single SQLite file. WAL mode, foreign keys on, busy timeout
  — all set via the DSN. Schema changes are versioned SQL migrations, embedded
  into the binary and applied on startup.
- **No ORM**: every query is explicit SQL in the `store` layer, one file per
  entity, with small `scan*` helpers.
- **Security-first defaults**: the flag never leaves the server through a
  player-facing shape; access is gated in the handlers; passwords are bcrypt;
  sessions are opaque random tokens in an `HttpOnly` cookie.

---

## Architecture

Packages live under `internal/`, each with a single responsibility:

```
config      ENV → Config (PORT, DB_PATH, FILES_DIR, ALLOWED_ORIGIN, COOKIE_*, ADMIN_*)
db          how we connect: Open (pragmas via DSN) + Migrate (embedded migrations)
store       what we read/write: one Store{db}, hand-written SQL, a file per entity
auth        password hashing, session tokens/TTL, the session cookie
files       path-traversal-safe file access (os.Root) behind the gated /files/* routes
middleware  RequireLogin, RequireAdmin, CORS, Logging, Recover; UserFromContext
respond     consistent JSON / error responses (dependency-free, avoids import cycles)
handlers    the endpoints (methods on Handlers{...}) and Routes() — the whole API map
```

Entry points:

- `cmd/server` — `config → db.Open → db.Migrate → bootstrap admin → Routes → ListenAndServe`.
- `cmd/seed` — dev-only loader for `seed/seed.json` (wipes content tables and
  resets autoincrement so level ids are deterministic).

Migrations are in `migrations/*.sql`, compiled into the binary via `embed.go`.

### Request lifecycle

The middleware chain wraps the router outermost-to-innermost:

```
Logging → Recover → CORS → ServeMux → handler
```

- **Logging** times the whole chain and records the final status.
- **Recover** turns a handler panic into a logged `500`.
- **CORS** echoes the single `ALLOWED_ORIGIN` and handles the `OPTIONS`
  preflight (which matches no route on its own).
- **RequireLogin** validates the session cookie, attaches the user to the
  request context, and applies **sliding expiration** (a session close to expiry
  is refreshed; an expired one is deleted and its cookie cleared).
- **RequireAdmin** layers on `RequireLogin`, then checks the role — so the order
  is `401` for no/invalid session, `403` for a valid non-admin.

The **access gate** for a specific level lives in the handler (it depends on the
id in the path), not in middleware.

---

## Data model

Six tables (`migrations/000001_init_schema.up.sql`):

| Table | Purpose | Notable constraints |
|-------|---------|---------------------|
| `users` | accounts | `username` unique; `role` defaults to `player` |
| `sessions` | login sessions | `token` PK; `user_id` → `users` **ON DELETE CASCADE** |
| `levels` | puzzles | `order_index` **unique** (ordering, with gaps); `unlocks_tool_id` → `tools` (**RESTRICT**: a referenced tool can't be deleted) |
| `hints` | per-level hints | `level_id` → `levels` **CASCADE**; ordered by `order_index` |
| `tools` | toolkit entries | `type` (`link`/`pdf`/`builtin`), `content` |
| `user_progress` | which levels a user solved | `user_id` & `level_id` → CASCADE; **`UNIQUE(user_id, level_id)`** makes recording a solve idempotent |

Derived concepts (computed, not stored):

- **Current level** = `COUNT(solved) + 1` — the player's ordinal position; a
  fresh account is level `1`.
- **Accessible levels** = the first `current` levels by `order_index` (every
  solved level plus the next unsolved one). Future levels are never returned.

---

## API

All responses are JSON. Errors use a consistent `{"error": "..."}` body. Player
and admin routes require the session cookie (`credentials` on the client);
admin routes additionally require `role == admin`.

### Public

| Method & path | Body | Result |
|---|---|---|
| `POST /api/register` | `{username, password}` | `201 {id, username}`; `409` taken; `400` validation |
| `POST /api/login` | `{username, password}` | `200 {id, username, role}` + sets cookie; `401` bad credentials |
| `POST /api/logout` | — | clears the session and cookie |
| `GET /health` | — | `200 {status: "ok"}` |

### Player (authenticated)

| Method & path | Result |
|---|---|
| `GET /api/me` | `{id, username, role, currentLevel}` |
| `GET /api/levels` | accessible levels: `[{id, title, solved}]` (no flag) |
| `GET /api/levels/{id}` | `{id, title, description, solved, files[]}`; `404` missing / `403` locked |
| `POST /api/levels/{id}/submit` | `{answer}` → `{correct: bool}` (records progress on success) |
| `GET /api/levels/{id}/hints` | `[{id, text}]`, ordered; same gate as the level |
| `GET /api/tools` | the player's unlocked tools: `[{id, type, title, description, content}]` |
| `GET /files/levels/{id}/{path...}` | a puzzle attachment, gated by level access |
| `GET /files/tools/{path...}` | a tool file, gated by whether the user unlocked a tool pointing at it |

### Admin (`role == admin`)

| Method & path | Notes |
|---|---|
| `GET/POST /api/admin/levels`, `PUT/DELETE /api/admin/levels/{id}` | full level **including the flag**; `409` duplicate `order_index`, `400` bad `unlocks_tool_id` |
| `GET/PUT /api/admin/levels/{id}/hints` | `PUT {hints: string[]}` replaces the whole ordered list |
| `GET/POST /api/admin/tools`, `PUT/DELETE /api/admin/tools/{id}` | `type` whitelist; deleting a referenced tool → `409` |
| `GET /api/admin/users`, `PUT /api/admin/users/{id}` | list with `current_level`; `PUT {role?, level?}` |
| `POST /api/admin/users/{id}/reset-password` | returns a one-time `{password}` and invalidates the user's sessions |
| `DELETE /api/admin/users/{id}` | self-demote / self-delete are blocked with `409` |

`Routes()` in `internal/handlers/routes.go` is the single source of truth for
this map.

---

## Design decisions worth knowing

- **The flag never leaks.** Player-facing structs (`LevelListItem`,
  `LevelDetail`) have no flag field and don't even `SELECT` it. Only
  `LevelForSubmit` (the submit path) and the role-gated admin surface
  (`AdminLevel`) read it. The `TestFlagNeverInPlayerResponses` test guards this
  boundary across every player endpoint.
- **Flag comparison** is case-insensitive and exact — both sides lowercased, **no
  trimming** (precise format is part of the challenge). The submit body field is
  `answer`, not `flag`.
- **Gated downloads** (`/files/*`) go through `internal/files.Open`, which uses
  `os.Root` (Go 1.24+) to confine a request path to its base directory — a
  crafted or encoded `..` path can neither escape nor reach a future level's
  files. The access gate runs before any disk access.
- **A level's files are discovered, not registered**: `GET /api/levels/{id}`
  lists whatever sits in `FILES_DIR/levels/{id}/`, so the frontend can render
  download links without a separate upload/registration step.
- **Password reset invalidates all of the user's sessions** — validation is
  token-based, so changing the hash alone wouldn't log anyone out.
- **First admin** is bootstrapped from `ADMIN_USERNAME` / `ADMIN_PASSWORD` at
  startup, idempotently (only if no admin exists). No secrets in the repo.

---

## Project layout

```
cmd/
  server/        entry point: wire config, db, migrations, admin bootstrap, server
  seed/          dev-only seed loader
internal/
  config/        environment configuration
  db/            connection (pragmas) + migration runner
  store/         SQL data layer (users, sessions, levels, hints, tools, progress)
  auth/          password, session token, cookie
  files/         path-traversal-safe file open/list
  middleware/    auth, CORS, logging, recover
  respond/       JSON response helpers
  handlers/      HTTP handlers + Routes()
migrations/      versioned schema (embedded)
seed/            seed.json (mock content for local dev)
embed.go         embeds migrations into the binary
```

---

## Local development

No `.env` is needed — the config defaults target local dev (SQLite at `./ctf.db`,
files under `./files`, CORS for `http://localhost:5173`).

```sh
go build ./... && go vet ./... && go test ./...   # build + checks + tests
go run ./cmd/server                                # starts on :8080
go run ./cmd/seed                                  # load mock content from seed/seed.json
```

### Testing conventions

- **Every store method** has unit tests against a freshly migrated database in a
  temp dir (`newTestStore`).
- **Handlers and middleware** are exercised end-to-end through the real router
  with `httptest` (see `internal/handlers/handlers_test.go`), including the
  flag-leak and access-gate guards.

### Configuration

All settings come from the environment — see [`.env.example`](./.env.example)
for the full list. The defaults are local-dev friendly; everything that matters
for a real deployment (CORS origin, cookie flags, admin bootstrap, storage
paths) is overridable there.
