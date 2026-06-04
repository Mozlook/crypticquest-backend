package auth

import (
	"crypto/rand"
	"encoding/base64"
	"time"
)

// SessionTTL is how long a freshly created session stays valid. The auth
// middleware extends it via sliding expiration while the player is active.
const SessionTTL = 30 * 24 * time.Hour

// GenerateSessionToken returns a cryptographically random, URL-safe session
// identifier: 32 bytes from crypto/rand, base64url-encoded without padding.
// crypto/rand (not math/rand) is essential — the token is the only thing
// standing between a request and an account.
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
