# syntax=docker/dockerfile:1

# ---- build stage -----------------------------------------------------------
# modernc.org/sqlite is pure Go, so CGO can stay off and we get a fully static
# binary that runs on a minimal base with no libc surprises.
FROM golang:1.26-alpine AS build
WORKDIR /src

# The base image may lag the exact patch in go.mod's `go` directive; auto lets
# the toolchain fetch the required version instead of failing with GOTOOLCHAIN=local.
ENV GOTOOLCHAIN=auto

# Download modules first so this layer caches unless go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- runtime stage ---------------------------------------------------------
# alpine (not distroless) so sqlite3 is on hand for `docker exec ... .backup`,
# plus a shell for debugging. ca-certificates/tzdata for future outbound calls
# and correct timestamps.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates sqlite tzdata \
    && adduser -D -u 10001 app

WORKDIR /app
COPY --from=build /out/server /app/server

# Migrations are embedded in the binary, so main() runs them on startup — the
# container needs no separate migrate step.
ENV PORT=8080 \
    DB_PATH=/data/ctf.db \
    FILES_DIR=/app/files

# /data is the DB volume mountpoint. /app/files is the mountpoint for the gated
# content bind mount — puzzle/tool files live only on the server, never baked
# into the image (so no one can pull answers/tools from the repo or image). Both
# owned by the non-root user so a fresh mount inherits writable ownership.
RUN mkdir -p /data /app/files && chown app:app /data /app/files
VOLUME ["/data"]
USER app
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/server"]
