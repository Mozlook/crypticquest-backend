package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"crypticquest/internal/middleware"
	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

type submitRequest struct {
	Answer string `json:"answer"`
}

// SubmitFlag handles POST /api/levels/{id}/submit: validate a submitted flag and
// record progress on success. Returns only {"correct": true|false} — never a
// hint about how close the answer was, to avoid teaching brute force.
func (h *Handlers) SubmitFlag(w http.ResponseWriter, r *http.Request) {
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

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	orderIndex, flag, err := h.store.LevelForSubmit(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "level not found")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not process submission")
		return
	}

	// Gate before comparing, so a locked level's flag can't be probed.
	accessible, err := h.store.IsLevelAccessible(user.ID, orderIndex)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not process submission")
		return
	}
	if !accessible {
		respond.Error(w, http.StatusForbidden, "level locked")
		return
	}

	// Lowercase both sides and compare exactly — no trimming, format matters.
	correct := strings.ToLower(req.Answer) == strings.ToLower(flag)
	if correct {
		if err := h.store.RecordSolved(user.ID, id); err != nil {
			respond.Error(w, http.StatusInternalServerError, "could not record progress")
			return
		}
	}

	respond.JSON(w, http.StatusOK, map[string]bool{"correct": correct})
}
