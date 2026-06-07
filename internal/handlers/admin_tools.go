package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

// allowedToolTypes is the whitelist for tool.type, enforced in Go (no DB CHECK).
// link = external URL, pdf = self-hosted material, builtin = in-app mini-tool.
var allowedToolTypes = map[string]bool{"link": true, "pdf": true, "builtin": true}

type adminToolRequest struct {
	Type             string `json:"type"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Content          string `json:"content"`
	UnlocksAtLevelID *int64 `json:"unlocks_at_level_id"`
}

// toInput validates the request and converts it to a store input, returning a
// client-facing (400) message on failure.
func (req adminToolRequest) toInput() (store.ToolInput, string) {
	if !allowedToolTypes[req.Type] {
		return store.ToolInput{}, "type must be one of: link, pdf, builtin"
	}
	if strings.TrimSpace(req.Title) == "" {
		return store.ToolInput{}, "title is required"
	}
	if len(req.Title) > maxTitleLen {
		return store.ToolInput{}, "title is too long"
	}
	if strings.TrimSpace(req.Content) == "" {
		return store.ToolInput{}, "content is required"
	}
	if len(req.Content) > maxToolContentLen {
		return store.ToolInput{}, "content is too long"
	}
	if len(req.Description) > maxToolDescLen {
		return store.ToolInput{}, "description is too long"
	}
	return store.ToolInput{
		Type:             req.Type,
		Title:            req.Title,
		Description:      req.Description,
		Content:          req.Content,
		UnlocksAtLevelID: req.UnlocksAtLevelID,
	}, ""
}

// AdminListTools handles GET /api/admin/tools: every tool.
func (h *Handlers) AdminListTools(w http.ResponseWriter, r *http.Request) {
	tools, err := h.store.ListAllTools()
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load tools")
		return
	}
	respond.JSON(w, http.StatusOK, tools)
}

// AdminCreateTool handles POST /api/admin/tools.
func (h *Handlers) AdminCreateTool(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeToolInput(w, r)
	if !ok {
		return
	}
	id, err := h.store.CreateTool(in)
	if err != nil {
		writeToolWriteErr(w, err)
		return
	}
	tool, err := h.store.ToolByID(id)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "tool created but could not be loaded")
		return
	}
	respond.JSON(w, http.StatusCreated, tool)
}

// AdminUpdateTool handles PUT /api/admin/tools/{id}.
func (h *Handlers) AdminUpdateTool(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	in, ok := decodeToolInput(w, r)
	if !ok {
		return
	}
	if err := h.store.UpdateTool(id, in); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "tool not found")
			return
		}
		writeToolWriteErr(w, err)
		return
	}
	tool, err := h.store.ToolByID(id)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "tool updated but could not be loaded")
		return
	}
	respond.JSON(w, http.StatusOK, tool)
}

// AdminDeleteTool handles DELETE /api/admin/tools/{id}. The unlock relation lives
// on the tool, so deleting one never orphans a level and is always allowed.
func (h *Handlers) AdminDeleteTool(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.DeleteTool(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "tool not found")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not delete tool")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeToolWriteErr maps the store's tool-write sentinels to HTTP statuses.
func writeToolWriteErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrInvalidReference) {
		respond.Error(w, http.StatusBadRequest, "unlocks_at_level_id references a non-existent level")
		return
	}
	respond.Error(w, http.StatusInternalServerError, "could not save tool")
}

// decodeToolInput decodes and validates a tool body, writing the 400 itself on
// failure and returning ok=false.
func decodeToolInput(w http.ResponseWriter, r *http.Request) (store.ToolInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req adminToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid JSON body")
		return store.ToolInput{}, false
	}
	in, msg := req.toInput()
	if msg != "" {
		respond.Error(w, http.StatusBadRequest, msg)
		return store.ToolInput{}, false
	}
	return in, true
}
