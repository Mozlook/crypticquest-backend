package crypticquest

import "embed"

// MigrationsFS holds the golang-migrate SQL files compiled into the binary,
// so the deploy stays a single self-contained executable (no migrations
// folder to ship alongside it).
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
