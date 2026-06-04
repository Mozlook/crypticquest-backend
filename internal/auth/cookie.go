package auth

import (
	"net/http"
	"strings"
	"time"
)

// SessionCookieName is the name of the cookie carrying the session token.
const SessionCookieName = "session"

// SessionCookie holds the cookie attributes derived from config once, so
// handlers set and clear the session cookie consistently without re-deriving
// flags. Build it at startup with NewSessionCookie and hand it to the handlers.
type SessionCookie struct {
	Domain   string
	Secure   bool
	SameSite http.SameSite
}

// NewSessionCookie builds the cookie settings from raw config values, mapping
// the SameSite string ("None"/"Lax"/"Strict") onto the net/http enum. Note:
// SameSite=None is only honoured by browsers together with Secure=true.
func NewSessionCookie(domain string, secure bool, sameSite string) SessionCookie {
	return SessionCookie{
		Domain:   domain,
		Secure:   secure,
		SameSite: parseSameSite(sameSite),
	}
}

func parseSameSite(s string) http.SameSite {
	switch strings.ToLower(s) {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
}

// Set writes the session cookie carrying token, expiring at expiresAt.
func (c SessionCookie) Set(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Domain:   c.Domain,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: c.SameSite,
	})
}

// Clear removes the session cookie on logout. The attributes (Path, Domain,
// flags) must match Set so the browser targets the same cookie; MaxAge=-1
// tells it to delete the cookie immediately.
func (c SessionCookie) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   c.Domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: c.SameSite,
	})
}
