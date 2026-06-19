// Package dav implements a minimal, dependency-free WebDAV gateway over a
// store.Backend. The namespace is flat: a single root collection containing
// every object as a member file. It supports the methods needed by common
// clients (rclone, davfs2, curl, macOS Finder, Windows Explorer):
//
//	OPTIONS, PROPFIND (Depth 0/1), GET/HEAD (with Range), PUT, DELETE,
//	MKCOL (no-op at root), COPY, MOVE, and LOCK/UNLOCK stubs.
//
// Authentication is HTTP Basic, validated through an injected Authenticator so
// this package stays free of any password-hashing dependency.
package dav

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gpix/pkg/store"
)

// Authenticator reports whether the given basic-auth credentials are valid.
type Authenticator func(user, pass string) bool

// Config configures a WebDAV server.
type Config struct {
	Listen   string // address to listen on, e.g. "127.0.0.1:8081"
	BasePath string // URL prefix the dav tree is mounted at; default "/"
	Realm    string // basic-auth realm; default "gpix"
}

// Server is a WebDAV server.
type Server struct {
	cfg     Config
	be      store.Backend
	auth    Authenticator
	log     *slog.Logger
	httpSrv *http.Server
}

// New constructs a WebDAV server. If auth is nil, requests are not
// authenticated (use only behind a trusted boundary / for tests).
func New(cfg Config, be store.Backend, auth Authenticator, log *slog.Logger) (*Server, error) {
	if log == nil {
		log = slog.Default()
	}
	if cfg.BasePath == "" {
		cfg.BasePath = "/"
	}
	if !strings.HasSuffix(cfg.BasePath, "/") {
		cfg.BasePath += "/"
	}
	if cfg.Realm == "" {
		cfg.Realm = "gpix"
	}
	s := &Server{cfg: cfg, be: be, auth: auth, log: log}
	s.httpSrv = &http.Server{
		Addr:              cfg.Listen,
		Handler:           s,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	return s, nil
}

// Handler exposes the raw http.Handler (useful for tests).
func (s *Server) Handler() http.Handler { return s }

// Run starts the server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("webdav gateway listening", "addr", s.cfg.Listen, "base", s.cfg.BasePath)
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

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("dav panic", "recover", rec)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}()

	if !s.checkAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="`+s.cfg.Realm+`"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	key, isRoot, ok := s.keyFromPath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case "OPTIONS":
		s.handleOptions(w, r)
	case "PROPFIND":
		s.handlePropfind(w, r, key, isRoot)
	case "GET", "HEAD":
		if isRoot {
			http.Error(w, "is a collection", http.StatusMethodNotAllowed)
			return
		}
		s.handleGet(w, r, key, r.Method == "HEAD")
	case "PUT":
		if isRoot {
			http.Error(w, "is a collection", http.StatusMethodNotAllowed)
			return
		}
		s.handlePut(w, r, key)
	case "DELETE":
		s.handleDelete(w, r, key, isRoot)
	case "MKCOL":
		// Flat namespace: accept directory creation as a no-op so clients that
		// pre-create folders can proceed.
		w.WriteHeader(http.StatusCreated)
	case "COPY":
		s.handleCopyMove(w, r, key, isRoot, false)
	case "MOVE":
		s.handleCopyMove(w, r, key, isRoot, true)
	case "LOCK":
		s.handleLock(w, r)
	case "UNLOCK":
		w.WriteHeader(http.StatusNoContent)
	case "PROPPATCH":
		// Pretend success: we don't persist dead properties.
		s.writeMultistatus(w, []responseXML{{
			Href:     s.href(key, isRoot),
			Propstat: []propstatXML{{Status: "HTTP/1.1 200 OK"}},
		}})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) checkAuth(r *http.Request) bool {
	if s.auth == nil {
		return true
	}
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	return s.auth(user, pass)
}

// keyFromPath maps a request path to an object key. It returns isRoot=true when
// the path addresses the root collection.
func (s *Server) keyFromPath(p string) (key string, isRoot bool, ok bool) {
	if !strings.HasPrefix(p, s.cfg.BasePath) {
		// Allow the base path without trailing slash too.
		if p+"/" == s.cfg.BasePath {
			return "", true, true
		}
		return "", false, false
	}
	rest := strings.TrimPrefix(p, s.cfg.BasePath)
	if rest == "" {
		return "", true, true
	}
	dec, err := url.PathUnescape(rest)
	if err != nil {
		return "", false, false
	}
	dec = strings.TrimSuffix(dec, "/")
	if dec == "" {
		return "", true, true
	}
	return dec, false, true
}

func (s *Server) href(key string, isRoot bool) string {
	if isRoot {
		return s.cfg.BasePath
	}
	// Encode each path segment but keep separators.
	segs := strings.Split(key, "/")
	for i, seg := range segs {
		segs[i] = url.PathEscape(seg)
	}
	return s.cfg.BasePath + strings.Join(segs, "/")
}

func (s *Server) handleOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("DAV", "1, 2")
	w.Header().Set("MS-Author-Via", "DAV")
	w.Header().Set("Allow", "OPTIONS, GET, HEAD, PUT, DELETE, PROPFIND, PROPPATCH, MKCOL, COPY, MOVE, LOCK, UNLOCK")
	w.WriteHeader(http.StatusOK)
}

// --- PROPFIND ---

func (s *Server) handlePropfind(w http.ResponseWriter, r *http.Request, key string, isRoot bool) {
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "infinity"
	}
	// Drain the request body (some clients send a <propfind> XML we ignore).
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))

	var responses []responseXML

	if isRoot {
		responses = append(responses, collectionResponse(s.cfg.BasePath))
		if depth != "0" {
			objs, err := s.be.List(r.Context())
			if err != nil {
				http.Error(w, "list: "+err.Error(), http.StatusInternalServerError)
				return
			}
			for _, o := range objs {
				responses = append(responses, s.objectResponse(o))
			}
		}
		s.writeMultistatus(w, responses)
		return
	}

	o, err := s.be.Stat(r.Context(), key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	responses = append(responses, s.objectResponse(o))
	s.writeMultistatus(w, responses)
}

func collectionResponse(href string) responseXML {
	return responseXML{
		Href: href,
		Propstat: []propstatXML{{
			Status: "HTTP/1.1 200 OK",
			Prop: propXML{
				DisplayName:  collectionName(href),
				ResourceType: &resourceTypeXML{Collection: &struct{}{}},
				LastModified: time.Now().UTC().Format(http.TimeFormat),
			},
		}},
	}
}

func collectionName(href string) string {
	href = strings.TrimSuffix(href, "/")
	if i := strings.LastIndexByte(href, '/'); i >= 0 {
		return href[i+1:]
	}
	return href
}

func (s *Server) objectResponse(o store.Object) responseXML {
	ct := o.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	mod := o.ModTime
	if mod.IsZero() {
		mod = time.Now()
	}
	name := o.Key
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return responseXML{
		Href: s.href(o.Key, false),
		Propstat: []propstatXML{{
			Status: "HTTP/1.1 200 OK",
			Prop: propXML{
				DisplayName:   name,
				ResourceType:  &resourceTypeXML{},
				ContentLength: o.Size,
				ContentType:   ct,
				LastModified:  mod.UTC().Format(http.TimeFormat),
				CreationDate:  mod.UTC().Format("2006-01-02T15:04:05Z"),
				ETag:          `"` + o.ETag + `"`,
			},
		}},
	}
}

func (s *Server) writeMultistatus(w http.ResponseWriter, responses []responseXML) {
	ms := multistatusXML{Responses: responses}
	out, err := xml.Marshal(ms)
	if err != nil {
		http.Error(w, "xml: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, xml.Header)
	_, _ = w.Write(out)
}

// --- GET / HEAD ---

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, key string, headOnly bool) {
	if headOnly {
		o, err := s.be.Stat(r.Context(), key)
		if err != nil {
			s.getError(w, err)
			return
		}
		s.setGetHeaders(w, o)
		if o.Size >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(o.Size, 10))
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	rc, o, err := s.be.Get(r.Context(), key)
	if err != nil {
		s.getError(w, err)
		return
	}
	defer rc.Close()
	s.setGetHeaders(w, o)

	if rng := r.Header.Get("Range"); rng != "" && o.Size >= 0 {
		start, length, ok := parseByteRange(rng, o.Size)
		if ok {
			if start > 0 {
				if _, err := io.CopyN(io.Discard, rc, start); err != nil {
					return
				}
			}
			w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
			w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(start+length-1, 10)+"/"+strconv.FormatInt(o.Size, 10))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.CopyN(w, rc, length)
			return
		}
	}

	if o.Size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(o.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func (s *Server) setGetHeaders(w http.ResponseWriter, o store.Object) {
	if o.ContentType != "" {
		w.Header().Set("Content-Type", o.ContentType)
	}
	if !o.ModTime.IsZero() {
		w.Header().Set("Last-Modified", o.ModTime.UTC().Format(http.TimeFormat))
	}
	if o.ETag != "" {
		w.Header().Set("ETag", `"`+o.ETag+`"`)
	}
	w.Header().Set("Accept-Ranges", "bytes")
}

func (s *Server) getError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// --- PUT / DELETE / COPY / MOVE ---

func (s *Server) handlePut(w http.ResponseWriter, r *http.Request, key string) {
	_, err := s.be.Put(r.Context(), key, r.Body, r.ContentLength, r.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, "put: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, key string, isRoot bool) {
	if isRoot {
		http.Error(w, "refusing to delete collection root", http.StatusForbidden)
		return
	}
	err := s.be.Delete(r.Context(), key)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		http.Error(w, "delete: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCopyMove(w http.ResponseWriter, r *http.Request, srcKey string, isRoot, move bool) {
	if isRoot {
		http.Error(w, "cannot copy/move collection root", http.StatusForbidden)
		return
	}
	dest := r.Header.Get("Destination")
	if dest == "" {
		http.Error(w, "missing Destination", http.StatusBadRequest)
		return
	}
	destKey, ok := s.destKey(dest)
	if !ok {
		http.Error(w, "bad Destination", http.StatusBadRequest)
		return
	}

	rc, _, err := s.be.Get(r.Context(), srcKey)
	if err != nil {
		s.getError(w, err)
		return
	}
	defer rc.Close()
	if _, err := s.be.Put(r.Context(), destKey, rc, -1, ""); err != nil {
		http.Error(w, "copy: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if move {
		if err := s.be.Delete(r.Context(), srcKey); err != nil && !errors.Is(err, store.ErrNotFound) {
			http.Error(w, "move-delete: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) destKey(dest string) (string, bool) {
	// Destination may be a full URL or an absolute path.
	if u, err := url.Parse(dest); err == nil && u.Path != "" {
		dest = u.Path
	}
	if !strings.HasPrefix(dest, s.cfg.BasePath) {
		return "", false
	}
	rest := strings.TrimPrefix(dest, s.cfg.BasePath)
	dec, err := url.PathUnescape(rest)
	if err != nil {
		return "", false
	}
	dec = strings.TrimSuffix(dec, "/")
	if dec == "" {
		return "", false
	}
	return dec, true
}

// --- LOCK (stub: grants an exclusive token without real bookkeeping) ---

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	tokBytes := make([]byte, 16)
	_, _ = rand.Read(tokBytes)
	token := "opaquelocktoken:" + hex.EncodeToString(tokBytes)

	body := `<?xml version="1.0" encoding="utf-8"?>
<D:prop xmlns:D="DAV:"><D:lockdiscovery><D:activelock>` +
		`<D:locktype><D:write/></D:locktype>` +
		`<D:lockscope><D:exclusive/></D:lockscope>` +
		`<D:depth>infinity</D:depth>` +
		`<D:timeout>Second-3600</D:timeout>` +
		`<D:locktoken><D:href>` + token + `</D:href></D:locktoken>` +
		`</D:activelock></D:lockdiscovery></D:prop>`

	w.Header().Set("Lock-Token", "<"+token+">")
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

func parseByteRange(header string, size int64) (start, length int64, ok bool) { //nolint:unparam
	const p = "bytes="
	if !strings.HasPrefix(header, p) {
		return 0, 0, false
	}
	spec := strings.TrimSpace(header[len(p):])
	if strings.ContainsRune(spec, ',') {
		return 0, 0, false
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, false
	}
	startStr := strings.TrimSpace(spec[:dash])
	endStr := strings.TrimSpace(spec[dash+1:])
	if startStr == "" {
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		return size - n, n, true
	}
	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end := size - 1
	if endStr != "" {
		end, err = strconv.ParseInt(endStr, 10, 64)
		if err != nil || end < start {
			return 0, 0, false
		}
	}
	if end >= size {
		end = size - 1
	}
	return start, end - start + 1, true
}
