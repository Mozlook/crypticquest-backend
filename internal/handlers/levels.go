package handlers

import (
	"net/http"

	"crypticquest/internal/middleware"
	"crypticquest/internal/respond"
)

// ListLevels handles GET /api/levels: the player's accessible levels (solved
// plus the next unsolved one), without flags. Future levels are not returned.
func (h *Handlers) ListLevels(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	levels, err := h.store.ListAccessibleLevels(user.ID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load levels")
		return
	}
	respond.JSON(w, http.StatusOK, levels)
}
