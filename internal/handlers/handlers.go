// Package handlers holds the HTTP handlers for the JSON API. Each endpoint is
// a method on Handlers, which carries the shared dependencies (data store,
// session-cookie settings) so the handlers stay free of global state.
package handlers

import (
	"crypticquest/internal/auth"
	"crypticquest/internal/store"
)

// Handlers bundles the dependencies shared by all HTTP handlers.
type Handlers struct {
	store          *store.Store
	cookie         auth.SessionCookie
	filesDir       string   // base directory for gated downloads (files/levels, files/tools)
	allowedOrigins []string // the CORS allowlist echoed for credentialed requests
}

// New constructs the handler set.
func New(st *store.Store, cookie auth.SessionCookie, filesDir string, allowedOrigins []string) *Handlers {
	return &Handlers{store: st, cookie: cookie, filesDir: filesDir, allowedOrigins: allowedOrigins}
}
