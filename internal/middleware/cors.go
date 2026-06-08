package middleware

import "net/http"

// CORS returns middleware that enables credentialed cross-origin requests from a
// set of trusted origins (the deployed frontends in prod, localhost in dev).
// With credentials the wildcard "*" is forbidden, so the request's origin is
// echoed back only when it is on the allowlist; any other origin gets no CORS
// headers and the browser blocks the response. Preflight (OPTIONS) requests are
// answered here with 204 and never reach the router.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Vary: Origin always, so a shared cache never serves a CORS response
			// to a different origin than it was built for.
			w.Header().Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			isPreflight := r.Method == http.MethodOptions &&
				r.Header.Get("Access-Control-Request-Method") != ""

			if origin != "" && allowed[origin] {
				h := w.Header()
				// Echo the request's origin (never "*"), which is what credentialed
				// CORS requires; it is on the allowlist, so this is safe.
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				if isPreflight {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					// Echo the requested headers, or default to Content-Type for a
					// plain JSON body.
					reqHeaders := r.Header.Get("Access-Control-Request-Headers")
					if reqHeaders == "" {
						reqHeaders = "Content-Type"
					}
					h.Set("Access-Control-Allow-Headers", reqHeaders)
					h.Set("Access-Control-Max-Age", "86400") // cache preflight 24h
				}
			}

			// Terminate every preflight here: 204 with the headers set above (or
			// none, for a disallowed origin — the browser then blocks the request).
			if isPreflight {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
