// Package middleware holds cross-cutting HTTP request wrappers: authentication
// now; CORS, request logging, and the admin-role gate later. Middleware are
// constructed as closures over their dependencies, so they stay decoupled from
// the handler set.
package middleware

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"crypticquest/internal/auth"
	"crypticquest/internal/respond"
	"crypticquest/internal/store"
)

// ctxKey is an unexported context-key type so values stored by this package
// can't collide with keys from other packages.
type ctxKey int

const ctxKeyUser ctxKey = iota

// UserFromContext returns the authenticated user attached by RequireLogin. The
// bool is false if the request did not pass through the middleware.
func UserFromContext(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(ctxKeyUser).(store.User)
	return u, ok
}

// RequireLogin returns middleware that admits only requests carrying a valid,
// unexpired session, placing the authenticated user in the request context for
// the downstream handler. An expired session is deleted on use (lazy cleanup)
// and its stale cookie cleared.
func RequireLogin(st *store.Store, cookie auth.SessionCookie) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(auth.SessionCookieName)
			if err != nil || c.Value == "" {
				respond.Error(w, http.StatusUnauthorized, "authentication required")
				return
			}

			sess, user, err := st.SessionByToken(c.Value)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					respond.Error(w, http.StatusUnauthorized, "authentication required")
					return
				}
				respond.Error(w, http.StatusInternalServerError, "could not verify session")
				return
			}

			if time.Now().After(sess.ExpiresAt) {
				if err := st.DeleteSession(sess.Token); err != nil {
					log.Printf("require-login: delete expired session: %v", err)
				}
				cookie.Clear(w)
				respond.Error(w, http.StatusUnauthorized, "session expired")
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyUser, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
