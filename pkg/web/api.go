package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"gpix/pkg/gpmc"
	"gpix/pkg/share"
)

// ---- JSON helpers ----------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// ---- auth middleware -------------------------------------------------------

// requireAPIToken authenticates a request via `Authorization: Bearer <token>`
// against the API-token store. On success it puts the token label in the
// request context (prefixed "api:") so downstream reuse of session-oriented
// helpers still works.
func (s *Server) requireAPIToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apitokens == nil {
			writeAPIError(w, http.StatusNotFound, "api disabled")
			return
		}
		tok := bearerToken(r)
		if tok == "" {
			writeAPIError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		t, ok := s.apitokens.Verify(r.Context(), tok)
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyUser, "api:"+t.Label)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// ---- endpoints -------------------------------------------------------------

// apiPing lets a client validate the server URL + token and discover features.
func (s *Server) apiPing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"app":        "gpix",
		"user":       strings.TrimPrefix(userFromCtx(r.Context()), "api:"),
		"encryption": s.crypt != nil && s.crypt.Enabled(),
		"features": map[string]bool{
			"upload": true,
			"shares": s.share != nil,
			"import": s.importer != nil,
		},
	})
}

type apiMediaItem struct {
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	Class     string    `json:"class"` // photo | video | doc | file
	Encrypted bool      `json:"encrypted"`
	Size      int64     `json:"size"`
	Mtime     time.Time `json:"mtime"`
	Thumb     string    `json:"thumb"`
	Original  string    `json:"original"`
}

// apiMediaList returns one page of library items using Google's native cursor
// pagination. This returns the first page immediately instead of listing the
// entire library up front (which made the first request hang on large
// libraries). `cursor` is the opaque token from the previous page's
// `nextCursor` (empty for the first page); `nextCursor` is empty when done.
func (s *Server) apiMediaList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cursor := r.URL.Query().Get("cursor")
	if allDigits(cursor) {
		// An older app build sent an integer offset here. That is not a valid
		// Google resume token (it makes lib-state 400 "invalid argument"), so
		// treat it as "first page" instead of erroring.
		cursor = ""
	}
	page, err := s.gp.ListPage(ctx, cursor)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	items := make([]apiMediaItem, 0, len(page.Items))
	for _, it := range page.Items {
		display, class, disguised := classifyItem(it.Filename, it.Kind)
		items = append(items, apiMediaItem{
			Key:       it.MediaKey,
			Name:      display,
			Class:     string(class),
			Encrypted: disguised && class != classFile,
			Size:      it.SizeBytes,
			Mtime:     it.Mtime,
			Thumb:     "/api/v1/media/" + it.MediaKey + "/thumb",
			Original:  "/api/v1/media/" + it.MediaKey + "/original",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"nextCursor": page.NextToken,
	})
}

// apiThumb serves a thumbnail (decrypted/rendered for disguised items). Reuses
// the same non-blocking thumbnail path as the web UI.
func (s *Server) apiThumb(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeAPIError(w, http.StatusBadRequest, "missing key")
		return
	}
	s.serveThumb(w, r, key, thumbSize(r))
}

// apiOriginal streams the decrypted original bytes; ?download=1 forces an
// attachment disposition.
func (s *Server) apiOriginal(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeAPIError(w, http.StatusBadRequest, "missing key")
		return
	}
	s.proxyDownload(w, r, key, r.URL.Query().Get("download") == "1")
}

// apiUpload accepts a multipart body (field "files", optional "quality") and
// uploads each file through the normal path (encryption/disguise/dedup apply).
func (s *Server) apiUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "multipart: "+err.Error())
		return
	}
	quality := gpmc.QualityOriginal
	uploadID := randID()
	results := make([]map[string]string, 0)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "part: "+err.Error())
			return
		}
		switch part.FormName() {
		case "quality":
			b, _ := io.ReadAll(io.LimitReader(part, 64))
			switch string(b) {
			case "saver":
				quality = gpmc.QualitySaver
			case "quota":
				quality = gpmc.QualityUseQuota
			}
		case "files":
			fn := part.FileName()
			if fn == "" {
				continue
			}
			name, status, key := parseUploadResult(s.uploadOne(r, part, fn, quality, uploadID))
			results = append(results, map[string]string{"name": name, "status": status, "key": key})
		}
	}
	if s.lib != nil {
		s.lib.Invalidate()
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// apiDelete moves an item to trash (soft delete).
func (s *Server) apiDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeAPIError(w, http.StatusBadRequest, "missing key")
		return
	}
	ctx, cancel := withTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.gp.DeleteByMediaKeys(ctx, []string{key}, false)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	if e := res[key]; e != nil {
		writeAPIError(w, http.StatusBadGateway, e.Error())
		return
	}
	if s.lib != nil {
		s.lib.Invalidate()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- shares ----------------------------------------------------------------

type apiShare struct {
	Token         string     `json:"token"`
	Title         string     `json:"title"`
	URL           string     `json:"url"`
	HasPassword   bool       `json:"hasPassword"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	MaxDownloads  int64      `json:"maxDownloads"`
	Downloads     int64      `json:"downloads"`
	AllowOriginal bool       `json:"allowOriginal"`
	CreatedAt     time.Time  `json:"createdAt"`
	Count         int        `json:"count"`
}

func (s *Server) shareURL(token string) string {
	return strings.TrimRight(s.cfg.ServerURL, "/") + "/s/" + token
}

func (s *Server) toAPIShare(sh share.Share) apiShare {
	var exp *time.Time
	if !sh.ExpiresAt.IsZero() {
		e := sh.ExpiresAt
		exp = &e
	}
	return apiShare{
		Token:         sh.Token,
		Title:         sh.Title,
		URL:           s.shareURL(sh.Token),
		HasPassword:   sh.HasPassword,
		ExpiresAt:     exp,
		MaxDownloads:  sh.MaxDownloads,
		Downloads:     sh.Downloads,
		AllowOriginal: sh.AllowOriginal,
		CreatedAt:     sh.CreatedAt,
		Count:         len(sh.Items),
	}
}

func (s *Server) apiSharesList(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		writeAPIError(w, http.StatusNotFound, "shares unavailable")
		return
	}
	ctx, cancel := withTimeout(r.Context(), 15*time.Second)
	defer cancel()
	list, err := s.share.List(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]apiShare, 0, len(list))
	for _, sh := range list {
		out = append(out, s.toAPIShare(sh))
	}
	writeJSON(w, http.StatusOK, map[string]any{"shares": out})
}

func (s *Server) apiShareCreate(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		writeAPIError(w, http.StatusNotFound, "shares unavailable")
		return
	}
	var body struct {
		Title         string   `json:"title"`
		Keys          []string `json:"keys"`
		Password      string   `json:"password"`
		ExpiresHours  int      `json:"expiresHours"`
		MaxDownloads  int64    `json:"maxDownloads"`
		AllowOriginal bool     `json:"allowOriginal"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	if len(body.Keys) == 0 {
		writeAPIError(w, http.StatusBadRequest, "keys required")
		return
	}
	items := make([]share.ShareItem, 0, len(body.Keys))
	for _, key := range body.Keys {
		name := key
		isVideo := false
		if s.lib != nil {
			if it, ok := s.lib.Peek(key); ok {
				display, class, _ := classifyItem(it.Filename, it.Kind)
				name = display
				isVideo = class == classVideo
			}
		}
		items = append(items, share.ShareItem{MediaKey: key, FileName: name, IsVideo: isVideo})
	}
	ctx, cancel := withTimeout(r.Context(), 15*time.Second)
	defer cancel()
	sh, err := s.share.Create(ctx, share.CreateParams{
		Title:         body.Title,
		Items:         items,
		Password:      body.Password,
		TTL:           time.Duration(body.ExpiresHours) * time.Hour,
		MaxDownloads:  body.MaxDownloads,
		AllowOriginal: body.AllowOriginal,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.toAPIShare(sh))
}

func (s *Server) apiShareRevoke(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		writeAPIError(w, http.StatusNotFound, "shares unavailable")
		return
	}
	token := r.PathValue("token")
	if token == "" {
		writeAPIError(w, http.StatusBadRequest, "missing token")
		return
	}
	ctx, cancel := withTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.share.Delete(ctx, token); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- Immich import ---------------------------------------------------------

func (s *Server) apiImportStatus(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		writeAPIError(w, http.StatusNotFound, "import unavailable")
		return
	}
	writeJSON(w, http.StatusOK, s.importer.Status())
}

func (s *Server) apiImportStart(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		writeAPIError(w, http.StatusNotFound, "import unavailable")
		return
	}
	var body struct {
		ServerURL string `json:"serverUrl"`
		APIKey    string `json:"apiKey"`
		Workers   int    `json:"workers"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	if body.ServerURL == "" || body.APIKey == "" {
		writeAPIError(w, http.StatusBadRequest, "serverUrl and apiKey required")
		return
	}
	if body.Workers <= 0 {
		body.Workers = 3
	}
	if err := s.importer.Start(body.ServerURL, body.APIKey, body.Workers); err != nil {
		writeAPIError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) apiImportStop(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		writeAPIError(w, http.StatusNotFound, "import unavailable")
		return
	}
	s.importer.Stop()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- small helpers ---------------------------------------------------------

// allDigits reports whether s is non-empty and only ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// parseUploadResult splits the tab-delimited line returned by uploadOne
// ("name\tstatus · mediaKey") into its parts.
func parseUploadResult(line string) (name, status, key string) {
	parts := strings.SplitN(line, "\t", 2)
	name = parts[0]
	if len(parts) < 2 {
		return name, "error", ""
	}
	rest := parts[1]
	if i := strings.LastIndex(rest, " · "); i >= 0 {
		return name, rest[:i], rest[i+len(" · "):]
	}
	return name, rest, ""
}
