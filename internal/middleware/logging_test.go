package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLog redirects the standard logger for the duration of fn and returns
// what was written.
func captureLog(fn func()) string {
	var buf bytes.Buffer
	old := log.Writer()
	flags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(old)
		log.SetFlags(flags)
	}()
	fn()
	return buf.String()
}

func TestLogging(t *testing.T) {
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("ok"))
	}))

	var rec *httptest.ResponseRecorder
	out := captureLog(func() {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/levels", nil))
	})

	if rec.Code != http.StatusCreated || rec.Body.String() != "ok" {
		t.Fatalf("response not passed through: %d %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(out, "POST /api/levels -> 201") {
		t.Fatalf("log line missing/wrong: %q", out)
	}
}

func TestLoggingDefaultStatus(t *testing.T) {
	// A handler that writes a body without WriteHeader -> status logged as 200.
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi"))
	}))
	out := captureLog(func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	})
	if !strings.Contains(out, "-> 200") {
		t.Fatalf("want default 200, got %q", out)
	}
}

func TestRecover(t *testing.T) {
	handler := Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	var rec *httptest.ResponseRecorder
	out := captureLog(func() {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/boom", nil))
	})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic should become 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Fatalf("body should be generic error, got %q", rec.Body.String())
	}
	if !strings.Contains(out, "panic recovered") || !strings.Contains(out, "boom") {
		t.Fatalf("panic should be logged with stack, got %q", out)
	}
}

func TestLoggingRecoverCompose(t *testing.T) {
	// Logging(Recover(panic)) -> the logged status is the 500 Recover wrote.
	handler := Logging(Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("kaboom")
	})))
	out := captureLog(func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	})
	if !strings.Contains(out, "GET /x -> 500") {
		t.Fatalf("logging should report the recovered 500, got %q", out)
	}
}
