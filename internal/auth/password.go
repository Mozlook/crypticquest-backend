// Package auth holds password hashing and (later) session/authentication
// helpers. Kept separate from the data layer so the same primitives are
// reused by registration, login, and the admin bootstrap.
package auth

import (
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/bcrypt"
)

// GenerateTempPassword returns a cryptographically random, URL-safe temporary
// password (18 bytes from crypto/rand, base64url without padding → 24 chars).
// Used by the admin password-reset flow; the plaintext is shown to the admin
// once and only its bcrypt hash is stored.
func GenerateTempPassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashPassword returns a bcrypt hash of the plaintext password, suitable for
// storing in users.password_hash. Never store or log the plaintext.
func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether plaintext matches the stored bcrypt hash.
// bcrypt re-derives the hash using the salt embedded in storedHash and compares
// in constant time, so there is no need to (and no way to) decrypt the hash.
func CheckPassword(storedHash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(plaintext)) == nil
}
