# crypticquest-backend

Go (`net/http`) + SQLite (`modernc.org/sqlite`, no cgo) backend for CrypticQuest.

## Local development

No `.env` needed — the config defaults target local dev.

```sh
go build ./... && go vet ./... && go test ./...   # build + checks + tests
go run ./cmd/server                                # starts on :8080
go run ./cmd/seed                                  # load mock content from seed/seed.json
```

## Deployment (Docker, behind a reverse proxy)

The container listens on `:8080` and is published only on loopback; the host's
reverse proxy (nginx/Caddy/Traefik) terminates HTTPS and proxies to it.

```sh
cp .env.example .env        # then edit: ALLOWED_ORIGIN, ADMIN_*, cookie flags
docker compose up -d --build
```

- **Migrations** run automatically on startup (embedded in the binary).
- **First admin** is bootstrapped from `ADMIN_USERNAME` / `ADMIN_PASSWORD` if no
  admin exists yet.
- The database persists in the named volume `cq-data`, mounted at `/data` as a
  **directory** (SQLite WAL writes `ctf.db` plus `-wal` / `-shm` siblings).

Reverse proxy must: terminate TLS, forward to `127.0.0.1:8080`, and preserve the
`Origin` header so CORS works. With `COOKIE_SECURE=true` + `COOKIE_SAMESITE=None`
the cross-site session cookie (Netlify frontend ↔ VPS backend) works over HTTPS.

## Configuration

All settings come from the environment — see [`.env.example`](./.env.example)
for the full list and prod-oriented values.

## Content files (puzzle attachments & tool PDFs)

The admin panel creates levels and tools (database rows) but **does not upload
files** — there is no upload endpoint. The gated `/files/*` routes serve from
`FILES_DIR`, which in production is a **read-only bind mount** from a host
directory (`CQ_FILES_DIR`, see `.env.example`). Those files live **only on the
server** — never in git or the image — so no one can pull answers/tools from the
repo instead of solving.

Layout under `CQ_FILES_DIR`:

```
levels/{id}/<file>   # puzzle attachments; {id} is the level's DB id
tools/<file>         # tool files; <file> must match the tool's `content` field
```

To add a file:

1. Create the level (or tool) in the admin panel first — a level's `{id}` is
   assigned by the database; a tool's `content` is the filename you type.
2. Copy the file onto the host into the right subdirectory, e.g.:

   ```sh
   ssh user@vps 'mkdir -p /srv/crypticquest/files/levels/7'
   scp message.txt    user@vps:/srv/crypticquest/files/levels/7/
   scp ascii-table.pdf user@vps:/srv/crypticquest/files/tools/
   ```

3. It's served immediately — no restart or rebuild. The app reads from disk per
   request and discovers a level's files on the fly.

Files must be **readable by the container user** (uid 10001); a world-readable
`chmod a+rX` on the tree is the simplest guarantee. The bind mount is read-only,
so the app can serve but never modify them.

## Backup & restore

SQLite is a single file, but **do not copy `ctf.db` while the app is running** —
a mid-WAL-write copy can be inconsistent. Two safe options:

**Online snapshot (no downtime)** — `sqlite3 .backup` is consistent against a
live WAL database:

```sh
docker exec crypticquest-backend \
  sqlite3 /data/ctf.db ".backup '/data/backup.db'"
docker cp crypticquest-backend:/data/backup.db ./ctf-backup-$(date +%F).db
docker exec crypticquest-backend rm /data/backup.db
```

**Cold copy (with downtime)** — stop first, then copy the whole volume directory:

```sh
docker compose stop backend
docker run --rm -v cq-data:/data -v "$PWD":/out alpine \
  sh -c 'cp /data/ctf.db* /out/'
docker compose start backend
```

**Restore:** stop the container, replace `ctf.db` in the volume and remove any
stale `ctf.db-wal` / `ctf.db-shm`, then start again.

## Updating

```sh
git pull
docker compose up -d --build      # rebuilds the image, recreates the container
```

The `cq-data` volume survives rebuilds and `docker compose down`; it is removed
only by an explicit `docker compose down -v` or `docker volume rm cq-data`.
