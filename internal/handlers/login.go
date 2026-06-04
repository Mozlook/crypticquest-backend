package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"crypticquest/internal/auth"
	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

// dummyHash is a valid bcrypt hash computed once at startup. When a login names
// a non-existent user we still run a bcrypt comparison against it, so a missing
// user and a wrong password take comparable time (mitigates user enumeration
// via response timing). The "_" ignores the error: bcrypt won't fail here.
var dummyHash, _ = auth.HashPassword("timing-equalizer-not-a-real-password")

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login handles POST /api/login: verify credentials, create a session, set the
// session cookie. On bad credentials it returns a single generic 401 for both
// unknown-username and wrong-password.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Username == "" || req.Password == "" {
		respond.Error(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := h.store.UserByUsername(req.Username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			auth.CheckPassword(dummyHash, req.Password) // equalize timing
			respond.Error(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not process login")
		return
	}

	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		respond.Error(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token, err := auth.GenerateSessionToken()
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not create session")
		return
	}
	expiresAt := time.Now().Add(auth.SessionTTL)
	if err := h.store.CreateSession(token, user.ID, expiresAt); err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not create session")
		return
	}

	h.cookie.Set(w, token, expiresAt)
	respond.JSON(w, http.StatusOK, map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"role":     user.Role,
	})
}
