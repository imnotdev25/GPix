// Package s3 implements a minimal, dependency-free S3-compatible HTTP gateway
// over a store.Backend. It speaks path-style addressing and authenticates
// requests with AWS Signature V4 (header and presigned-URL forms), supporting
// UNSIGNED-PAYLOAD and STREAMING-AWS4-HMAC-SHA256-PAYLOAD bodies.
//
// Supported operations: ListBuckets, ListObjects (v1 & v2), HeadBucket,
// HeadObject, GetObject (with Range), PutObject, DeleteObject and the batch
// DeleteObjects (POST ?delete). Multipart upload, ACLs, versioning, tagging and
// bucket creation are intentionally not implemented; the gateway exposes a
// single fixed bucket.
package s3

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gpix/pkg/store"
)

// CredentialProvider resolves the secret access key for a presented access key
// id. Implementations must be safe for concurrent use; returning ok=false means
// the access key is unknown.
type CredentialProvider interface {
	Lookup(accessKey string) (secret string, ok bool)
}

// staticCreds is a single fixed access/secret pair.
type staticCreds struct{ access, secret string }

func (c staticCreds) Lookup(accessKey string) (string, bool) {
	if c.access == "" || accessKey != c.access {
		return "", false
	}
	return c.secret, true
}

// Config configures an S3 gateway server.
type Config struct {
	Listen string // address to listen on, e.g. "127.0.0.1:9000"
	Bucket string // the single bucket name exposed, e.g. "gpix"
	Region string // advertised region (informational); default "us-east-1"

	// Credentials, if set, resolves SigV4 secrets dynamically (preferred — lets
	// keys rotate at runtime). If nil, AccessKey/SecretKey below are used as a
	// single static credential. If neither is set, authentication is disabled
	// (intended only for tests behind a trusted boundary).
	Credentials CredentialProvider
	AccessKey   string
	SecretKey   string
}

// Server is an S3-compatible HTTP server.
type Server struct {
	cfg         Config
	be          store.Backend
	log         *slog.Logger
	ver         *verifier
	authEnabled bool
	httpSrv     *http.Server
}

// New constructs a Server. It returns an error if required configuration is
// missing.
func New(cfg Config, be store.Backend, log *slog.Logger) (*Server, error) {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "gpix"
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	var provider CredentialProvider
	authEnabled := false
	switch {
	case cfg.Credentials != nil:
		provider = cfg.Credentials
		authEnabled = true
	case cfg.AccessKey != "":
		provider = staticCreds{access: cfg.AccessKey, secret: cfg.SecretKey}
		authEnabled = true
	default:
		provider = staticCreds{}
	}

	s := &Server{
		cfg:         cfg,
		be:          be,
		log:         log,
		ver:         &verifier{provider: provider},
		authEnabled: authEnabled,
	}
	s.httpSrv = &http.Server{
		Addr:              cfg.Listen,
		Handler:           s,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	return s, nil
}

// Handler exposes the raw http.Handler (useful for tests via httptest).
func (s *Server) Handler() http.Handler { return s }

// Run starts the server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("s3 gateway listening", "addr", s.cfg.Listen, "bucket", s.cfg.Bucket)
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// ServeHTTP routes and authenticates every request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("s3 panic", "recover", rec)
			writeError(w, r, errInternal("internal error"))
		}
	}()

	if s.authEnabled {
		if e := s.ver.verify(r); e != nil {
			s.log.Debug("s3 auth failed", "code", e.Code, "path", r.URL.Path)
			writeError(w, r, *e)
			return
		}
	}

	bucket, key := splitPath(r.URL.Path)

	switch {
	case bucket == "":
		// Service-level request.
		if r.Method == http.MethodGet {
			s.handleListBuckets(w, r)
			return
		}
		writeError(w, r, errMethodNotAllowed)
		return

	case key == "":
		// Bucket-level request.
		if bucket != s.cfg.Bucket {
			writeError(w, r, errNoSuchBucket(bucket))
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleListObjects(w, r)
		case http.MethodHead:
			s.handleHeadBucket(w, r)
		case http.MethodPost:
			if _, ok := r.URL.Query()["delete"]; ok {
				s.handleDeleteObjects(w, r)
				return
			}
			writeError(w, r, errNotImplemented)
		case http.MethodPut:
			// CreateBucket: accept only the configured bucket name.
			w.Header().Set("Location", "/"+bucket)
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			// DeleteBucket on a fixed bucket is a no-op refusal.
			writeError(w, r, errMethodNotAllowed)
		default:
			writeError(w, r, errMethodNotAllowed)
		}
		return

	default:
		// Object-level request.
		if bucket != s.cfg.Bucket {
			writeError(w, r, errNoSuchBucket(bucket))
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleGetObject(w, r, key)
		case http.MethodHead:
			s.handleHeadObject(w, r, key)
		case http.MethodPut:
			s.handlePutObject(w, r, key)
		case http.MethodDelete:
			s.handleDeleteObject(w, r, key)
		default:
			writeError(w, r, errMethodNotAllowed)
		}
		return
	}
}

// splitPath splits a path-style request path "/bucket/key..." into bucket and
// key components. The key keeps its internal slashes. Both may be empty.
func splitPath(p string) (bucket, key string) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", ""
	}
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i], p[i+1:]
	}
	return p, ""
}

func requestID(r *http.Request) string {
	if v := r.Header.Get("x-amz-request-id"); v != "" {
		return v
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}
