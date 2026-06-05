package handlers

import (
	"errors"
	"net/http"
	"path"
	"path/filepath"
	"strconv"

	"crypticquest/internal/files"
	"crypticquest/internal/middleware"
	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

// ServeLevelFile handles GET /files/levels/{id}/{path...}: a puzzle input file,
// gated by level access exactly like GetLevel/GetHints (404 if the level does
// not exist, 403 if locked). The gate runs before any disk access; files.Open
// then confines {path...} to files/levels/{id}, so a guessed or crafted URL can
// neither escape the directory nor reach a future level's files.
func (h *Handlers) ServeLevelFile(w http.ResponseWriter, r *http.Request) {
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
		respond.Error(w, http.StatusInternalServerError, "could not serve file")
		return
	}

	accessible, err := h.store.IsLevelAccessible(user.ID, level.OrderIndex)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not serve file")
		return
	}
	if !accessible {
		respond.Error(w, http.StatusForbidden, "level locked")
		return
	}

	base := filepath.Join(h.filesDir, "levels", strconv.FormatInt(id, 10))
	h.serveFile(w, r, base, r.PathValue("path"))
}

// ServeToolFile handles GET /files/tools/{path...}: a toolkit material (PDF),
// gated by whether the user has unlocked a tool pointing at this file. Unlike
// the level route there is no id to look up — the gate is the path itself, so a
// path that matches no earned tool is refused with 403 (which also avoids
// revealing whether a future tool's file exists on disk).
func (h *Handlers) ServeToolFile(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	reqPath := r.PathValue("path")
	unlocked, err := h.store.IsToolFileUnlocked(user.ID, reqPath)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not serve file")
		return
	}
	if !unlocked {
		respond.Error(w, http.StatusForbidden, "tool locked")
		return
	}

	base := filepath.Join(h.filesDir, "tools")
	h.serveFile(w, r, base, reqPath)
}

// serveFile opens reqPath confined to base and streams it. Shared by the gated
// /files/* handlers; the access gate is the caller's job, this only enforces
// path confinement and streams the bytes (with content-type, Last-Modified and
// Range handled by http.ServeContent).
func (h *Handlers) serveFile(w http.ResponseWriter, r *http.Request, base, reqPath string) {
	f, err := files.Open(base, reqPath)
	if err != nil {
		if errors.Is(err, files.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "file not found")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not serve file")
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not serve file")
		return
	}

	// path.Base (not filepath.Base): reqPath is a URL path, always slash-separated.
	http.ServeContent(w, r, path.Base(reqPath), info.ModTime(), f)
}
