package handlers

import (
	"net/http"

	"crypticquest/internal/middleware"
	"crypticquest/internal/respond"
)

// Me handles GET /api/me: the logged-in player's data plus current level. The
// frontend calls this on startup. Mounted behind RequireLogin, so the user is
// always present in context.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		// Defensive: should never happen behind RequireLogin.
		respond.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	level, err := h.store.CurrentLevel(user.ID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load progress")
		return
	}

	respond.JSON(w, http.StatusOK, map[string]any{
		"id":           user.ID,
		"username":     user.Username,
		"role":         user.Role,
		"currentLevel": level,
	})
}
