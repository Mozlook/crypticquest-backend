package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"crypticquest/internal/auth"
	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

const (
	minUsernameLen = 3
	maxUsernameLen = 32
	minPasswordLen = 8
	// bcrypt hashes only the first 72 bytes; cap here so longer passwords are
	// rejected rather than silently truncated.
	maxPasswordLen = 72
)

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Register handles POST /api/register: validate input, hash the password, and
// create a player account. Does not log the user in — that is login's job.
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	// Cap the request body so a malicious client can't stream a huge payload.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if l := len(req.Username); l < minUsernameLen || l > maxUsernameLen {
		respond.Error(w, http.StatusBadRequest, "username must be 3-32 characters")
		return
	}
	if l := len(req.Password); l < minPasswordLen || l > maxPasswordLen {
		respond.Error(w, http.StatusBadRequest, "password must be 8-72 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not process password")
		return
	}

	id, err := h.store.CreateUser(req.Username, hash)
	if err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			respond.Error(w, http.StatusConflict, "username already taken")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not create account")
		return
	}

	respond.JSON(w, http.StatusCreated, map[string]any{
		"id":       id,
		"username": req.Username,
	})
}
