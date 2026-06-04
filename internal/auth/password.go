// Package auth holds password hashing and (later) session/authentication
// helpers. Kept separate from the data layer so the same primitives are
// reused by registration, login, and the admin bootstrap.
package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of the plaintext password, suitable for
// storing in users.password_hash. Never store or log the plaintext.
func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
