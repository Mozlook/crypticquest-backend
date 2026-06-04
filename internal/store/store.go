// Package store is the data-access layer: hand-written SQL over a *sql.DB,
// grouped by entity (one file per entity). No ORM, no query builder — queries
// stay explicit and visible. internal/db owns the connection and migrations;
// store owns reads and writes against it.
package store

import "database/sql"

// Store is the single data-access dependency handed to the HTTP handlers.
type Store struct {
	db *sql.DB
}

// New wraps an already-opened database in a Store.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}
