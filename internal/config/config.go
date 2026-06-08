// Package config loads runtime configuration from environment variables,
// falling back to sensible local-development defaults.
package config

import (
	"log"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the backend.
type Config struct {
	// Port is the TCP port the HTTP server listens on (without a colon).
	Port string
	// DBPath is the filesystem path to the SQLite database file.
	DBPath string
	// FilesDir is the base directory holding gated downloads, with the layout
	// files/levels/{id}/ and files/tools/. Served only through the authorization
	// gate, never a bare FileServer.
	FilesDir string
	// AllowedOrigins is the set of frontend origins permitted by CORS, parsed
	// from a comma-separated ALLOWED_ORIGIN (e.g. "https://app.example.com,
	// https://app.netlify.app"). With credentialed requests the wildcard "*"
	// is forbidden, so each must be an explicit origin.
	AllowedOrigins []string

	// Session cookie settings.
	CookieDomain   string // empty = host-only cookie (fine for local dev)
	CookieSecure   bool   // true in production (HTTPS); false for local http
	CookieSameSite string // "None" (cross-site, prod) / "Lax" / "Strict"

	// First-admin bootstrap. If both are set and no admin exists yet, an admin
	// account is created at startup. Leave empty to skip (e.g. once set up).
	AdminUsername string
	AdminPassword string
}

// Load reads configuration from the environment, applying local defaults.
func Load() Config {
	return Config{
		Port:           getEnv("PORT", "8080"),
		DBPath:         getEnv("DB_PATH", "ctf.db"),
		FilesDir:       getEnv("FILES_DIR", "files"),
		AllowedOrigins: getEnvList("ALLOWED_ORIGIN", []string{"http://localhost:5173"}),
		CookieDomain:   getEnv("COOKIE_DOMAIN", ""),
		CookieSecure:   getEnvBool("COOKIE_SECURE", false),
		CookieSameSite: getEnv("COOKIE_SAMESITE", "Lax"),
		AdminUsername:  getEnv("ADMIN_USERNAME", ""),
		AdminPassword:  getEnv("ADMIN_PASSWORD", ""),
	}
}

// getEnv returns the value of the environment variable, or fallback if unset.
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// getEnvList parses a comma-separated environment variable into a slice,
// trimming spaces and dropping empties. Returns fallback if unset or empty.
func getEnvList(key string, fallback []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

// getEnvBool parses a boolean environment variable, or returns fallback.
func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		log.Printf("config: invalid bool for %s=%q, using default %v", key, v, fallback)
		return fallback
	}
	return parsed
}
