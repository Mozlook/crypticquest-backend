package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

// writeJSON serializes v as JSON with the given status code. Sets the header
// before writing the status, as required by net/http.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status/headers are already sent, so we can only log.
		log.Printf("writeJSON: encoding response: %v", err)
	}
}

// writeError sends a consistent JSON error body: {"error": "..."}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
