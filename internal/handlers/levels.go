package handlers

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

	"crypticquest/internal/files"
	"crypticquest/internal/middleware"
	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

// levelDetailResponse is the single-level player view: the store's LevelDetail
// (no flag) plus the names of any files served for this level, so the frontend
// can render download links without guessing filenames. Files are discovered
// from files/levels/{id}/ — dropping a file there makes it appear.
type levelDetailResponse struct {
	store.LevelDetail
	Files []string `json:"files"`
}

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

// GetLevel handles GET /api/levels/{id}: details of one level (no flag), gated
// by access. 404 if the level does not exist, 403 if it exists but is locked.
func (h *Handlers) GetLevel(w http.ResponseWriter, r *http.Request) {
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

	level, err := h.store.LevelByID(user.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "level not found")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not load level")
		return
	}

	accessible, err := h.store.IsLevelAccessible(user.ID, level.OrderIndex)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load level")
		return
	}
	if !accessible {
		respond.Error(w, http.StatusForbidden, "level locked")
		return
	}

	levelFiles, err := files.List(filepath.Join(h.filesDir, "levels", strconv.FormatInt(id, 10)))
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load level")
		return
	}

	respond.JSON(w, http.StatusOK, levelDetailResponse{LevelDetail: level, Files: levelFiles})
}
