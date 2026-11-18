package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRedactPath(t *testing.T) {
	cases := map[string]string{
		"/s/abcdef123456":               "/s/abcd…",
		"/s/abcdef123456/raw/2":         "/s/abcd…/raw/2",
		"/stream/tok3n-value":           "/stream/tok3…",
		"/api/v1/shares/zzzzzzzz":       "/api/v1/shares/zzzz…",
		"/s/ab":                         "/s/ab…",
		"/s/":                           "/s/",
		"/api/v1/media":                 "/api/v1/media",
		"/api/v1/media/KEY/thumb":       "/api/v1/media/KEY/thumb",
		"/share/revoke/secretsecret":    "/share/revoke/secr…",
		"/raw/0123456789/whatever/else": "/raw/0123…/whatever/else",
	}
	for in, want := range cases {
		if got := redactPath(in); got != want {
			t.Errorf("redactPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRequestLevel(t *testing.T) {
	rec := func() *statusRecorder { return &statusRecorder{ResponseWriter: httptest.NewRecorder()} }
	get := httptest.NewRequest(http.MethodGet, "/api/v1/media", nil)
	post := httptest.NewRequest(http.MethodPost, "/api/v1/albums", nil)
	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	orig := httptest.NewRequest(http.MethodGet, "/api/v1/media/K/original", nil)

	cases := []struct {
		name   string
		r      *http.Request
		status int
		dur    time.Duration
		want   slog.Level
	}{
		{"fast read", get, 200, time.Millisecond, slog.LevelDebug},
		{"write", post, 200, time.Millisecond, slog.LevelInfo},
		{"server error", get, 500, 0, slog.LevelError},
		{"still loading", get, 503, 0, slog.LevelWarn},
		{"signed-out check", me, 401, 0, slog.LevelDebug},
		{"unauthorized", get, 401, 0, slog.LevelWarn},
		{"not found", get, 404, 0, slog.LevelInfo},
		{"slow read", get, 200, 12 * time.Second, slog.LevelWarn},
		{"slow download", orig, 200, 60 * time.Second, slog.LevelDebug},
	}
	for _, c := range cases {
		if got := requestLevel(c.r, c.status, c.dur, rec()); got != c.want {
			t.Errorf("%s: level %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLogRequestLine(t *testing.T) {
	var buf bytes.Buffer
	s := &Server{log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	h := s.logRequest(s.recoverPanic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = withUser(r, "me@example.com")
		if userFromCtx(r.Context()) != "me@example.com" {
			t.Error("user not in context")
		}
		if _, ok := w.(http.Flusher); !ok {
			t.Error("response writer lost http.Flusher")
		}
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	})))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/s/secrettoken/download", nil))

	if w.Header().Get("X-Request-Id") == "" {
		t.Error("no X-Request-Id header")
	}
	line := buf.String()
	for _, want := range []string{"status=418", "bytes=2", "user=me@example.com", "path=/s/secr…/download", "method=POST"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line %q is missing %q", line, want)
		}
	}
	if strings.Contains(line, "secrettoken") {
		t.Errorf("log line leaks the share token: %q", line)
	}
}

func TestPanicIsLoggedAs500(t *testing.T) {
	var buf bytes.Buffer
	s := &Server{log: slog.New(slog.NewTextHandler(&buf, nil))}
	h := s.logRequest(s.recoverPanic(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/media", nil))
	if w.Code != 500 {
		t.Fatalf("status %d, want 500", w.Code)
	}
	if !strings.Contains(buf.String(), "msg=panic") || !strings.Contains(buf.String(), "status=500") {
		t.Errorf("expected a panic line and a 500 request line, got:\n%s", buf.String())
	}
}
