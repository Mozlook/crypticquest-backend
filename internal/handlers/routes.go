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
	requireAdmin := middleware.RequireAdmin(h.store, h.cookie)

	mux.HandleFunc("GET /health", health)

	// Auth (public)
	mux.HandleFunc("POST /api/register", h.Register)
	mux.HandleFunc("POST /api/login", h.Login)
	mux.HandleFunc("POST /api/logout", h.Logout)

	// Authenticated
	mux.Handle("GET /api/me", requireLogin(http.HandlerFunc(h.Me)))
	mux.Handle("GET /api/levels", requireLogin(http.HandlerFunc(h.ListLevels)))
	mux.Handle("GET /api/levels/{id}", requireLogin(http.HandlerFunc(h.GetLevel)))
	mux.Handle("GET /api/levels/{id}/hints", requireLogin(http.HandlerFunc(h.GetHints)))
	mux.Handle("POST /api/levels/{id}/submit", requireLogin(http.HandlerFunc(h.SubmitFlag)))
	mux.Handle("GET /api/tools", requireLogin(http.HandlerFunc(h.ListTools)))

	// Gated downloads (outside /api/, but still authenticated for the gate)
	mux.Handle("GET /files/levels/{id}/{path...}", requireLogin(http.HandlerFunc(h.ServeLevelFile)))
	mux.Handle("GET /files/tools/{path...}", requireLogin(http.HandlerFunc(h.ServeToolFile)))

	// Admin (logged in AND role == admin)
	mux.Handle("GET /api/admin/levels", requireAdmin(http.HandlerFunc(h.AdminListLevels)))
	mux.Handle("POST /api/admin/levels", requireAdmin(http.HandlerFunc(h.AdminCreateLevel)))
	mux.Handle("PUT /api/admin/levels/{id}", requireAdmin(http.HandlerFunc(h.AdminUpdateLevel)))
	mux.Handle("DELETE /api/admin/levels/{id}", requireAdmin(http.HandlerFunc(h.AdminDeleteLevel)))
	mux.Handle("GET /api/admin/levels/{id}/hints", requireAdmin(http.HandlerFunc(h.AdminGetHints)))
	mux.Handle("PUT /api/admin/levels/{id}/hints", requireAdmin(http.HandlerFunc(h.AdminReplaceHints)))
	mux.Handle("GET /api/admin/tools", requireAdmin(http.HandlerFunc(h.AdminListTools)))
	mux.Handle("POST /api/admin/tools", requireAdmin(http.HandlerFunc(h.AdminCreateTool)))
	mux.Handle("PUT /api/admin/tools/{id}", requireAdmin(http.HandlerFunc(h.AdminUpdateTool)))
	mux.Handle("DELETE /api/admin/tools/{id}", requireAdmin(http.HandlerFunc(h.AdminDeleteTool)))
	mux.Handle("GET /api/admin/users", requireAdmin(http.HandlerFunc(h.AdminListUsers)))
	mux.Handle("PUT /api/admin/users/{id}", requireAdmin(http.HandlerFunc(h.AdminUpdateUser)))
	mux.Handle("POST /api/admin/users/{id}/reset-password", requireAdmin(http.HandlerFunc(h.AdminResetPassword)))
	mux.Handle("DELETE /api/admin/users/{id}", requireAdmin(http.HandlerFunc(h.AdminDeleteUser)))

	// Outermost to innermost: Logging (times the whole chain and sees the final
	// status) → Recover (turns a handler panic into a logged 500) → CORS (covers
	// OPTIONS preflight, which matches no route on its own) → router.
	return middleware.Logging(middleware.Recover(middleware.CORS(h.allowedOrigins)(mux)))
}

// health is a simple liveness check.
func health(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
