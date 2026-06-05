package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

// adminHintsRequest is the body for replacing a level's hints. The slice order is
// the hint order — there is no per-hint id or index to send.
type adminHintsRequest struct {
	Hints []string `json:"hints"`
}

// AdminGetHints handles GET /api/admin/levels/{id}/hints: the level's hints in
// order, for populating the editor. 404 if the level does not exist.
func (h *Handlers) AdminGetHints(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if _, err := h.store.AdminLevelByID(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "level not found")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not load hints")
		return
	}
	hints, err := h.store.HintsForLevel(id)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load hints")
		return
	}
	respond.JSON(w, http.StatusOK, hints)
}

// AdminReplaceHints handles PUT /api/admin/levels/{id}/hints: replace the whole
// ordered hint list (add/remove/reorder in one call). An empty list clears them.
func (h *Handlers) AdminReplaceHints(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req adminHintsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	for _, text := range req.Hints {
		if strings.TrimSpace(text) == "" {
			respond.Error(w, http.StatusBadRequest, "hint text must not be empty")
			return
		}
	}

	if err := h.store.ReplaceHints(id, req.Hints); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "level not found")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not save hints")
		return
	}

	hints, err := h.store.HintsForLevel(id)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "hints saved but could not be loaded")
		return
	}
	respond.JSON(w, http.StatusOK, hints)
}
