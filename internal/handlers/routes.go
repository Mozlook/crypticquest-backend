package handlers

import (
	"net/http"

	"crypticquest/internal/middleware"
	"crypticquest/internal/respond"
)

// Routes builds the HTTP router with every endpoint registered and returns it
// as the server's root handler. Keeping routing here, next to the handlers,
// leaves main() thin and gives one place to read the whole API surface.
func (h *Handlers) Routes() http.Handler {
	mux := http.NewServeMux()

	requireLogin := middleware.RequireLogin(h.store, h.cookie)

	mux.HandleFunc("GET /health", health)

	// Auth (public)
	mux.HandleFunc("POST /api/register", h.Register)
	mux.HandleFunc("POST /api/login", h.Login)
	mux.HandleFunc("POST /api/logout", h.Logout)

	// Authenticated
	mux.Handle("GET /api/me", requireLogin(http.HandlerFunc(h.Me)))
	mux.Handle("GET /api/levels", requireLogin(http.HandlerFunc(h.ListLevels)))
	mux.Handle("GET /api/levels/{id}", requireLogin(http.HandlerFunc(h.GetLevel)))
	mux.Handle("POST /api/levels/{id}/submit", requireLogin(http.HandlerFunc(h.SubmitFlag)))

	return mux
}

// health is a simple liveness check.
func health(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
