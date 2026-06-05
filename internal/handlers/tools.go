package handlers

import (
	"net/http"

	"crypticquest/internal/middleware"
	"crypticquest/internal/respond"
)

// ListTools handles GET /api/tools: the player's unlocked toolkit — the tools
// awarded by levels they have solved. No id in the path, so there is no per-item
// access gate here; UnlockedTools already filters to what the user has earned.
func (h *Handlers) ListTools(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	tools, err := h.store.UnlockedTools(user.ID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load tools")
		return
	}
	respond.JSON(w, http.StatusOK, tools)
}
