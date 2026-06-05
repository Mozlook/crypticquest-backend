package middleware

import "net/http"

// CORS returns middleware that enables credentialed cross-origin requests from a
// single trusted origin (the Netlify frontend in prod, localhost in dev). With
// credentials the wildcard "*" is forbidden, so the allowed origin is echoed
// back only when it matches exactly; any other origin gets no CORS headers and
// the browser blocks the response. Preflight (OPTIONS) requests are answered here
// with 204 and never reach the router.
func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Vary: Origin always, so a shared cache never serves a CORS response
			// to a different origin than it was built for.
			w.Header().Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			isPreflight := r.Method == http.MethodOptions &&
				r.Header.Get("Access-Control-Request-Method") != ""

			if origin != "" && origin == allowedOrigin {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", allowedOrigin)
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
