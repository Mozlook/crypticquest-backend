package handlers

import "net/http"

// Routes builds the HTTP router with every endpoint registered and returns it
// as the server's root handler. Keeping routing here, next to the handlers,
// leaves main() thin and gives one place to read the whole API surface.
func (h *Handlers) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", health)

	// Auth
	mux.HandleFunc("POST /api/register", h.Register)
	mux.HandleFunc("POST /api/login", h.Login)
	mux.HandleFunc("POST /api/logout", h.Logout)

	return mux
}

// health is a simple liveness check.
func health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
