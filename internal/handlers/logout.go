package handlers

import (
	"log"
	"net/http"

	"crypticquest/internal/auth"
)

// Logout handles POST /api/logout: delete the current session row and clear the
// cookie. Idempotent — succeeds even without a valid session cookie.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookieName); err == nil && c.Value != "" {
		if err := h.store.DeleteSession(c.Value); err != nil {
			// The cookie is cleared regardless, so a stale token is harmless;
			// log for visibility but don't fail the request.
			log.Printf("logout: delete session: %v", err)
		}
	}
	h.cookie.Clear(w)
	w.WriteHeader(http.StatusNoContent)
}
