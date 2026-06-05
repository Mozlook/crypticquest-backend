package middleware

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"crypticquest/internal/respond"
)

// statusRecorder wraps a ResponseWriter to remember the status code (and whether
// anything was written) so the logging middleware can report it. The status
// defaults to 200, matching net/http's behaviour when a handler writes a body
// without calling WriteHeader.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.written {
		r.status = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.status = http.StatusOK
		r.written = true
	}
	return r.ResponseWriter.Write(b)
}

// Logging logs one line per request: method, path, final status, and duration.
// It is the outermost middleware so it times the whole chain and observes the
// status that Recover writes on a panic.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

// Recover turns a panic in any downstream handler into a 500 JSON error instead
// of crashing the server (or dropping the connection). The stack is logged for
// debugging; the client only sees a generic message.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				respond.Error(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
