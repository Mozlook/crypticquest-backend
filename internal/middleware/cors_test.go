package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS(t *testing.T) {
	const allowed = "https://app.example.com"

	var ran bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	})
	handler := CORS(allowed)(next)

	do := func(method, origin, reqMethod string) *httptest.ResponseRecorder {
		ran = false
		r := httptest.NewRequest(method, "/api/levels", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if reqMethod != "" {
			r.Header.Set("Access-Control-Request-Method", reqMethod)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return rec
	}

	t.Run("allowed origin echoes headers and continues", func(t *testing.T) {
		rec := do(http.MethodGet, allowed, "")
		if !ran {
			t.Fatal("handler should run for a normal request")
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowed {
			t.Fatalf("ACAO = %q, want %q", got, allowed)
		}
		if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Fatal("missing Allow-Credentials")
		}
		if rec.Header().Get("Vary") != "Origin" {
			t.Fatalf("Vary = %q, want Origin", rec.Header().Get("Vary"))
		}
	})

	t.Run("disallowed origin gets no CORS headers", func(t *testing.T) {
		rec := do(http.MethodGet, "https://evil.example.com", "")
		if !ran {
			t.Fatal("handler still runs; the browser, not the server, blocks the read")
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("must not echo a disallowed origin")
		}
	})

	t.Run("no origin (same-origin/curl) passes through", func(t *testing.T) {
		rec := do(http.MethodGet, "", "")
		if !ran || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("expected pass-through with no CORS headers, ran=%v", ran)
		}
	})

	t.Run("allowed preflight -> 204 with methods, no handler", func(t *testing.T) {
		rec := do(http.MethodOptions, allowed, http.MethodPost)
		if ran {
			t.Fatal("preflight must not reach the handler")
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("preflight status = %d, want 204", rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != allowed {
			t.Fatal("preflight missing ACAO")
		}
		if rec.Header().Get("Access-Control-Allow-Methods") == "" {
			t.Fatal("preflight missing Allow-Methods")
		}
		if rec.Header().Get("Access-Control-Max-Age") == "" {
			t.Fatal("preflight missing Max-Age")
		}
	})

	t.Run("disallowed preflight -> 204 without ACAO", func(t *testing.T) {
		rec := do(http.MethodOptions, "https://evil.example.com", http.MethodPost)
		if ran || rec.Code != http.StatusNoContent {
			t.Fatalf("want 204 no-run, got %d ran=%v", rec.Code, ran)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("disallowed preflight must not echo origin")
		}
	})

	t.Run("requested headers are echoed", func(t *testing.T) {
		ran = false
		r := httptest.NewRequest(http.MethodOptions, "/api/login", nil)
		r.Header.Set("Origin", allowed)
		r.Header.Set("Access-Control-Request-Method", http.MethodPost)
		r.Header.Set("Access-Control-Request-Headers", "Content-Type, X-Custom")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, X-Custom" {
			t.Fatalf("Allow-Headers = %q", got)
		}
	})
}
