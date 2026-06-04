package main

import (
	"database/sql"
	"log"
	"net/http"

	"crypticquest"
	"crypticquest/internal/auth"
	"crypticquest/internal/config"
	"crypticquest/internal/db"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database, crypticquest.MigrationsFS); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Printf("database ready at %s (migrations applied)", cfg.DBPath)

	bootstrapAdmin(database, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)

	addr := ":" + cfg.Port
	log.Printf("CrypticQuest backend listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// bootstrapAdmin creates the first admin account from ADMIN_USERNAME /
// ADMIN_PASSWORD when configured and no admin exists yet. Non-fatal: a failure
// here is logged but still lets the server serve players.
func bootstrapAdmin(database *sql.DB, cfg config.Config) {
	if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
		return
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		log.Printf("admin bootstrap: hashing failed: %v", err)
		return
	}
	created, err := db.EnsureAdmin(database, cfg.AdminUsername, hash)
	switch {
	case err != nil:
		log.Printf("admin bootstrap: %v", err)
	case created:
		log.Printf("admin bootstrap: created admin %q", cfg.AdminUsername)
	default:
		log.Printf("admin bootstrap: an admin already exists, skipping")
	}
}

// healthHandler responds with a simple JSON liveness check.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
