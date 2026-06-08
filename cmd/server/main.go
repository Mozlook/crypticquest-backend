package main

import (
	"log"
	"net/http"

	"crypticquest"
	"crypticquest/internal/auth"
	"crypticquest/internal/config"
	"crypticquest/internal/db"
	"crypticquest/internal/handlers"
	"crypticquest/internal/store"
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

	st := store.New(database)
	bootstrapAdmin(st, cfg)

	cookie := auth.NewSessionCookie(cfg.CookieDomain, cfg.CookieSecure, cfg.CookieSameSite)
	h := handlers.New(st, cookie, cfg.FilesDir, cfg.AllowedOrigins)

	addr := ":" + cfg.Port
	log.Printf("CrypticQuest backend listening on %s", addr)
	if err := http.ListenAndServe(addr, h.Routes()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// bootstrapAdmin creates the first admin account from ADMIN_USERNAME /
// ADMIN_PASSWORD when configured and no admin exists yet. Non-fatal: a failure
// here is logged but still lets the server serve players.
func bootstrapAdmin(st *store.Store, cfg config.Config) {
	if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
		return
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		log.Printf("admin bootstrap: hashing failed: %v", err)
		return
	}
	created, err := st.EnsureAdmin(cfg.AdminUsername, hash)
	switch {
	case err != nil:
		log.Printf("admin bootstrap: %v", err)
	case created:
		log.Printf("admin bootstrap: created admin %q", cfg.AdminUsername)
	default:
		log.Printf("admin bootstrap: an admin already exists, skipping")
	}
}
