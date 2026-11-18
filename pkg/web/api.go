package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
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

// requireAPIToken authenticates a request either with `Authorization: Bearer
// <token>` (native clients) or with the browser session cookie (the built-in
// SPA, which is same-origin). On success the caller's identity goes in the
// request context so session-oriented helpers keep working.
func (s *Server) requireAPIToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if tok := bearerToken(r); tok != "" {
			if s.apitokens == nil {
				writeAPIError(w, http.StatusNotFound, "api tokens unavailable")
				return
			}
			t, ok := s.apitokens.Verify(r.Context(), tok)
			if !ok {
				writeAPIError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			next.ServeHTTP(w, withUser(r, "api:"+t.Label))
			return
		}
		// Session cookie (SPA). Cookies are SameSite=Lax, so cross-site POSTs
		// don't carry them; same-origin fetches from the SPA do.
		if c, err := r.Cookie(sessionCookieName); err == nil {
			if user, verr := s.verifySession(c.Value); verr == nil {
				next.ServeHTTP(w, withUser(r, user))
				return
			}
		}
		writeAPIError(w, http.StatusUnauthorized, "authentication required")
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
	Key          string    `json:"key"`
	Name         string    `json:"name"`
	Class        string    `json:"class"` // photo | video | doc | file
	Encrypted    bool      `json:"encrypted"`
	Size         int64     `json:"size"`
	Mtime        time.Time `json:"mtime"`
	Thumb        string    `json:"thumb"`
	Original     string    `json:"original"`
	ThumbPending bool      `json:"thumbPending,omitempty"` // gpix is still rendering the thumbnail
	Tags         []string  `json:"tags,omitempty"`
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
	var mtime time.Time
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
		case "mtime":
			// The file's modification time from the client (ms since the
			// epoch), used when the file has no capture date of its own.
			b, _ := io.ReadAll(io.LimitReader(part, 32))
			if ms, perr := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); perr == nil && ms > 0 {
				mtime = time.UnixMilli(ms)
			}
		case "files":
			fn := part.FileName()
			if fn == "" {
				continue
			}
			name, status, key := parseUploadResult(s.uploadOne(r, part, fn, quality, uploadID, mtime))
			results = append(results, map[string]string{"name": name, "status": status, "key": key})
			mtime = time.Time{} // applies only to the file right after it
		}
	}
	// New items reach the listing cache through the upload hook (see main.go).
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
	res, err := s.gpc().DeleteByMediaKeys(ctx, []string{key}, false)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	if e := res[key]; e != nil {
		writeAPIError(w, http.StatusBadGateway, e.Error())
		return
	}
	if lib := s.libc(); lib != nil {
		lib.Remove(key)
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
	DirectURL     string     `json:"directUrl,omitempty"` // starts the download (a .zip for several items)
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
		DirectURL:     s.directURL(sh),
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
		if lib := s.libc(); lib != nil {
			if it, ok := lib.Peek(key); ok {
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

// apiImportGoogleStart imports another Google account's library (given its
// GP_AUTH_DATA) into the active account.
func (s *Server) apiImportGoogleStart(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		writeAPIError(w, http.StatusNotFound, "import unavailable")
		return
	}
	var body struct {
		AuthData  string `json:"authData"`
		AccountID int64  `json:"accountId"` // a linked account to copy from, instead of authData
		Workers   int    `json:"workers"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	authData := strings.TrimSpace(body.AuthData)
	if body.AccountID > 0 {
		if body.AccountID == s.accts.ActiveID() {
			writeAPIError(w, http.StatusBadRequest, "that's the account gpix is using now; pick another account to copy from")
			return
		}
		ad, err := s.accts.AuthData(r.Context(), body.AccountID)
		if err != nil {
			writeAPIError(w, http.StatusNotFound, "no such linked account")
			return
		}
		authData = ad
	}
	if authData == "" {
		writeAPIError(w, http.StatusBadRequest, "choose a linked account or paste its GP_AUTH_DATA")
		return
	}
	if body.Workers <= 0 {
		body.Workers = 3
	}
	if err := s.importer.StartGoogle(authData, body.Workers); err != nil {
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
