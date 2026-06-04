// Package respond writes consistent JSON HTTP responses. It is intentionally
// dependency-free so any layer (handlers, middleware) can use it without
// creating an import cycle.
package respond

import (
	"encoding/json"
	"log"
	"net/http"
)

// JSON serializes v as JSON with the given status code. Sets the header before
// writing the status, as required by net/http.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status/headers are already sent, so we can only log.
		log.Printf("respond.JSON: encoding response: %v", err)
	}
}

// Error sends a consistent JSON error body: {"error": "..."}.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}
