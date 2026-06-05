package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"crypticquest/internal/auth"
	"crypticquest/internal/middleware"
	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

// adminUserUpdateRequest is the body for PUT /api/admin/users/{id}. Both fields
// are optional pointers: nil means "leave unchanged". Role changes the account's
// role; Level rewrites progress (simple mode) so current level becomes Level.
type adminUserUpdateRequest struct {
	Role  *string `json:"role"`
	Level *int    `json:"level"`
}

// AdminListUsers handles GET /api/admin/users: every account with role,
// registration date, and current level.
func (h *Handlers) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListUsers()
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not load users")
		return
	}
	respond.JSON(w, http.StatusOK, users)
}

// AdminUpdateUser handles PUT /api/admin/users/{id}: change role and/or progress.
// Guards against an admin demoting themselves, which would otherwise risk locking
// the last admin out of the panel.
func (h *Handlers) AdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	actor, ok := middleware.UserFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req adminUserUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Role != nil {
		if *req.Role != store.RolePlayer && *req.Role != store.RoleAdmin {
			respond.Error(w, http.StatusBadRequest, "role must be 'player' or 'admin'")
			return
		}
		if id == actor.ID && *req.Role != store.RoleAdmin {
			respond.Error(w, http.StatusConflict, "you cannot remove your own admin role")
			return
		}
	}
	if req.Level != nil && *req.Level < 1 {
		respond.Error(w, http.StatusBadRequest, "level must be a positive integer")
		return
	}

	if req.Role != nil {
		if err := h.store.SetUserRole(id, *req.Role); err != nil {
			if writeUserNotFound(w, err) {
				return
			}
			respond.Error(w, http.StatusInternalServerError, "could not update role")
			return
		}
	}
	if req.Level != nil {
		if err := h.store.SetUserProgressToLevel(id, *req.Level); err != nil {
			if writeUserNotFound(w, err) {
				return
			}
			respond.Error(w, http.StatusInternalServerError, "could not update progress")
			return
		}
	}

	user, err := h.store.AdminUserByID(id)
	if err != nil {
		if writeUserNotFound(w, err) {
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not load user")
		return
	}
	respond.JSON(w, http.StatusOK, user)
}

// AdminResetPassword handles POST /api/admin/users/{id}/reset-password: generate
// a random temporary password, store only its hash, and return the plaintext
// once so the admin can hand it to the player.
func (h *Handlers) AdminResetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	temp, err := auth.GenerateTempPassword()
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not generate password")
		return
	}
	hash, err := auth.HashPassword(temp)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	if err := h.store.UpdatePasswordHash(id, hash); err != nil {
		if writeUserNotFound(w, err) {
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	// Invalidate the user's existing sessions: a reset must log out anyone holding
	// the old credentials, otherwise a live (possibly compromised) session would
	// survive the change.
	if err := h.store.DeleteUserSessions(id); err != nil {
		respond.Error(w, http.StatusInternalServerError, "password reset but sessions could not be cleared")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"password": temp})
}

// AdminDeleteUser handles DELETE /api/admin/users/{id}. Sessions and progress
// cascade away. An admin cannot delete their own account (self-lockout guard).
func (h *Handlers) AdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	actor, ok := middleware.UserFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if id == actor.ID {
		respond.Error(w, http.StatusConflict, "you cannot delete your own account")
		return
	}
	if err := h.store.DeleteUser(id); err != nil {
		if writeUserNotFound(w, err) {
			return
		}
		respond.Error(w, http.StatusInternalServerError, "could not delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeUserNotFound writes a 404 and returns true when err is ErrNotFound,
// letting callers keep their error handling flat.
func writeUserNotFound(w http.ResponseWriter, err error) bool {
	if errors.Is(err, store.ErrNotFound) {
		respond.Error(w, http.StatusNotFound, "user not found")
		return true
	}
	return false
}
