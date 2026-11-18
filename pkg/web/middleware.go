package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return
		}
		user, err := s.verifySession(c.Value)
		if err != nil {
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, withUser(r, user))
	}
}

// corsAPI adds permissive CORS headers to the /api/v1 responses and answers
// preflight OPTIONS requests directly, so the Flutter *web* build (and any
// browser client) can call the token-authenticated API cross-origin. The API
// authenticates with a Bearer token and sets no cookies, and this does NOT send
// Access-Control-Allow-Credentials, so a cross-origin site still cannot act
// without already holding the token.
func (s *Server) corsAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

// ---- request logging ---------------------------------------------------------

// reqInfo travels in the request context so the auth middleware, which runs
// deeper in the chain, can tell the request logger who the caller was.
type reqInfo struct {
	id   string
	user string
}

const ctxKeyReq ctxKey = 2

func reqInfoFrom(ctx context.Context) *reqInfo {
	ri, _ := ctx.Value(ctxKeyReq).(*reqInfo)
	return ri
}

// withUser puts the signed-in user in the request context and in the log line.
func withUser(r *http.Request, user string) *http.Request {
	if ri := reqInfoFrom(r.Context()); ri != nil {
		ri.user = user
	}
	return r.WithContext(context.WithValue(r.Context(), ctxKeyUser, user))
}

// statusRecorder captures the status and size of a response. It keeps
// streaming (Flush) and sendfile (ReadFrom) working for the handlers below it.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (sr *statusRecorder) WriteHeader(code int) {
	if sr.status == 0 && code >= 200 {
		sr.status = code
	}
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(p []byte) (int, error) {
	if sr.status == 0 {
		sr.status = http.StatusOK
	}
	n, err := sr.ResponseWriter.Write(p)
	sr.bytes += int64(n)
	return n, err
}

func (sr *statusRecorder) ReadFrom(src io.Reader) (int64, error) {
	if sr.status == 0 {
		sr.status = http.StatusOK
	}
	var n int64
	var err error
	if rf, ok := sr.ResponseWriter.(io.ReaderFrom); ok {
		n, err = rf.ReadFrom(src)
	} else {
		n, err = io.Copy(sr.ResponseWriter, src)
	}
	sr.bytes += n
	return n, err
}

func (sr *statusRecorder) Flush() {
	if sr.status == 0 {
		sr.status = http.StatusOK
	}
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (sr *statusRecorder) Unwrap() http.ResponseWriter { return sr.ResponseWriter }

// logRequest writes one line per request: method, path (secrets hidden),
// status, size, duration, user and a request id (also sent back as
// X-Request-Id, so a user can quote it). Routine reads log at debug level;
// writes, client errors, slow requests and server errors are louder.
func (s *Server) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ri := &reqInfo{id: requestID(r)}
		w.Header().Set("X-Request-Id", ri.id)
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), ctxKeyReq, ri)))

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		dur := time.Since(start)
		lvl := requestLevel(r, status, dur, rec)
		if !s.log.Enabled(r.Context(), lvl) {
			return
		}
		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", redactPath(r.URL.Path)),
			slog.Int("status", status),
			slog.Int64("bytes", rec.bytes),
			slog.Int64("ms", dur.Milliseconds()),
			slog.String("ip", clientIP(r)),
			slog.String("req", ri.id),
		}
		if ri.user != "" {
			attrs = append(attrs, slog.String("user", ri.user))
		}
		s.log.LogAttrs(context.Background(), lvl, "http", attrs...)
	})
}

// requestID reuses a proxy's X-Request-Id when it looks sane, else makes one.
func requestID(r *http.Request) string {
	if v := r.Header.Get("X-Request-Id"); v != "" && len(v) <= 64 && strings.Trim(v, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.") == "" {
		return v
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// clientIP is the connection's address, plus what a reverse proxy says in
// X-Forwarded-For. It's only for the log, so the header isn't trusted further.
func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		if first = strings.TrimSpace(first); first != "" && first != ip {
			return first + " via " + ip
		}
	}
	return ip
}

// slowRequest is how long an ordinary request may take before it's a warning.
const slowRequest = 10 * time.Second

func requestLevel(r *http.Request, status int, dur time.Duration, rec *statusRecorder) slog.Level {
	switch {
	case status == http.StatusServiceUnavailable:
		return slog.LevelWarn // e.g. the library is still loading
	case status >= 500:
		return slog.LevelError
	case status == http.StatusUnauthorized && r.URL.Path == "/api/v1/auth/me":
		return slog.LevelDebug // the app asking "am I signed in?"
	case status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests:
		return slog.LevelWarn
	case status >= 400:
		return slog.LevelInfo
	case dur >= slowRequest && !longRunning(r, rec):
		return slog.LevelWarn
	case r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions:
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

// longRunning reports responses that are slow by nature: streams, big
// downloads and uploads.
func longRunning(r *http.Request, rec *statusRecorder) bool {
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		return true
	}
	if rec.bytes > 8<<20 || r.ContentLength > 8<<20 || r.ContentLength < 0 && r.Method == http.MethodPost {
		return true
	}
	p := r.URL.Path
	return strings.HasPrefix(p, "/stream/") || strings.HasPrefix(p, "/raw/") ||
		strings.HasSuffix(p, "/original") || strings.HasSuffix(p, "/download") ||
		strings.HasPrefix(p, "/api/upload-progress/")
}

// Path prefixes whose next segment is a secret (share and signed media links).
var secretPathPrefixes = []string{
	"/s/",
	"/stream/",
	"/raw/",
	"/share/revoke/",
	"/api/v1/shares/",
}

// redactPath keeps share tokens and signed-link tokens out of the log: only
// their first 4 characters are kept, enough to tell links apart.
func redactPath(p string) string {
	for _, pre := range secretPathPrefixes {
		if !strings.HasPrefix(p, pre) {
			continue
		}
		rest := p[len(pre):]
		tok, tail, hasTail := strings.Cut(rest, "/")
		if tok == "" {
			return p
		}
		if len(tok) > 4 {
			tok = tok[:4]
		}
		out := pre + tok + "…"
		if hasTail {
			out += "/" + tail
		}
		return out
	}
	return p
}

func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec) // the handler meant to abort the response
			}
			id := ""
			if ri := reqInfoFrom(r.Context()); ri != nil {
				id = ri.id
			}
			s.log.Error("panic", "method", r.Method, "path", redactPath(r.URL.Path), "req", id,
				"recover", rec, "stack", string(debug.Stack()))
			http.Error(w, "internal error", http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}
