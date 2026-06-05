package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

// adminLevelRequest is the JSON body for creating/updating a level. unlocks_tool_id
// is a pointer so an omitted/null value means "unlocks nothing" rather than 0.
type adminLevelRequest struct {
	OrderIndex    int    `json:"order_index"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Flag          string `json:"flag"`
	UnlocksToolID *int64 `json:"unlocks_tool_id"`
}

// toInput validates the request and converts it to a store input. The error
// message is client-facing (400).
func (req adminLevelRequest) toInput() (store.AdminLevelInput, string) {
	if req.OrderIndex <= 0 {
		return store.AdminLevelInput{}, "order_index must be a positive integer"
	}
	if strings.TrimSpace(req.Title) == "" {
		return store.AdminLevelInput{}, "title is required"
	}
	if strings.TrimSpace(req.Description) == "" {
		return store.AdminLevelInput{}, "description is required"
	}
	if req.Flag == "" {
		return store.AdminLevelInput{}, "flag is required"
	}
	return store.AdminLevelInput{
		OrderIndex:    req.OrderIndex,
		Title:         req.Title,
		Description:   req.Description,
		Flag:          req.Flag,
		UnlocksToolID: req.UnlocksToolID,
	}, ""
}

// AdminListLevels handles GET /api/admin/levels: every level, flag included.
func (h *Handlers) AdminListLevels(w http.ResponseWriter, r *http.Request) {
	levels, err := h.store.ListAllLevels()
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load levels")
		return
	}
	respond.JSON(w, http.StatusOK, levels)
}

// AdminCreateLevel handles POST /api/admin/levels.
func (h *Handlers) AdminCreateLevel(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeLevelInput(w, r)
	if !ok {
		return
	}
	id, err := h.store.CreateLevel(in)
	if err != nil {
		writeLevelWriteErr(w, err)
		return
	}
	level, err := h.store.AdminLevelByID(id)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "level created but could not be loaded")
		return
	}
	respond.JSON(w, http.StatusCreated, level)
}

// AdminUpdateLevel handles PUT /api/admin/levels/{id}.
func (h *Handlers) AdminUpdateLevel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	in, ok := decodeLevelInput(w, r)
	if !ok {
		return
	}
	if err := h.store.UpdateLevel(id, in); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "level not found")
			return
		}
		writeLevelWriteErr(w, err)
		return
	}
	level, err := h.store.AdminLevelByID(id)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "level updated but could not be loaded")
		return
	}
	respond.JSON(w, http.StatusOK, level)
}

// AdminDeleteLevel handles DELETE /api/admin/levels/{id}.
func (h *Handlers) AdminDeleteLevel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.DeleteLevel(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "level not found")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not delete level")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeLevelInput decodes and validates a level body, writing the 400 itself on
// failure and returning ok=false.
func decodeLevelInput(w http.ResponseWriter, r *http.Request) (store.AdminLevelInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req adminLevelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid JSON body")
		return store.AdminLevelInput{}, false
	}
	in, msg := req.toInput()
	if msg != "" {
		respond.Error(w, http.StatusBadRequest, msg)
		return store.AdminLevelInput{}, false
	}
	return in, true
}

// writeLevelWriteErr maps the store's constraint sentinels to HTTP statuses.
func writeLevelWriteErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrOrderIndexTaken):
		respond.Error(w, http.StatusConflict, "order_index already in use")
	case errors.Is(err, store.ErrInvalidReference):
		respond.Error(w, http.StatusBadRequest, "unlocks_tool_id references a non-existent tool")
	default:
		respond.Error(w, http.StatusInternalServerError, "could not save level")
	}
}

// parseID reads the {id} path value, writing a 400 on failure.
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}
