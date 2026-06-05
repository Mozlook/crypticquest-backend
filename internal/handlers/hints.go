package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"crypticquest/internal/middleware"
	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

// GetHints handles GET /api/levels/{id}/hints: all hints for one level, ordered,
// gated by access exactly like GetLevel (404 if the level does not exist, 403 if
// it exists but is locked). Every hint is returned at once; the reveal logic
// (timers) lives on the frontend.
func (h *Handlers) GetHints(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid level id")
		return
	}

	// Reuse LevelByID for the existence check (404) and its order_index, then the
	// positional gate (403) — same path GetLevel walks.
	level, err := h.store.LevelByID(user.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "level not found")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not load hints")
		return
	}

	accessible, err := h.store.IsLevelAccessible(user.ID, level.OrderIndex)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load hints")
		return
	}
	if !accessible {
		respond.Error(w, http.StatusForbidden, "level locked")
		return
	}

	hints, err := h.store.HintsForLevel(id)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load hints")
		return
	}
	respond.JSON(w, http.StatusOK, hints)
}
