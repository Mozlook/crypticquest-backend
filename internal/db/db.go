// Package db handles opening the SQLite database (with the mandatory
// per-connection pragmas) and applying schema migrations.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// Open opens the SQLite database at path with the pragmas this app requires
// on every connection: foreign_keys ON (so REFERENCES/CASCADE actually fire),
// WAL journal mode (better concurrent-write behaviour), and a busy timeout so
// a momentarily-locked DB waits instead of erroring immediately.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		path,
	)

	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return database, nil
}

// Migrate applies all pending "up" migrations from the given filesystem
// (expected to contain a "migrations" directory of golang-migrate .sql files).
// It is safe to call on every startup: a fully-migrated DB is a no-op.
func Migrate(database *sql.DB, migrationsFS fs.FS) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("load migration files: %w", err)
	}

	driver, err := migratesqlite.WithInstance(database, &migratesqlite.Config{})
	if err != nil {
		return fmt.Errorf("create migrate driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
