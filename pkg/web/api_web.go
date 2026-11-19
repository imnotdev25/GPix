package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gpix/pkg/share"
)

// JSON endpoints for what the server-rendered pages used to do: linking
// accounts, importing decryption keys, video streaming, and the public share
// page. The web app (ui/) is the only interface now.

func decodeBody(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func accountID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// ---- accounts ------------------------------------------------------------------

// apiAccountAdd links another Google account: {label, authData}.
func (s *Server) apiAccountAdd(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Label    string `json:"label"`
		AuthData string `json:"authData"`
	}
	if err := decodeBody(r, &b); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json")
		return
	}
	authData := strings.TrimSpace(b.AuthData)
	if authData == "" {
		writeAPIError(w, http.StatusBadRequest, "GP_AUTH_DATA is required")
		return
	}
	a, err := s.accts.Add(r.Context(), strings.TrimSpace(b.Label), authData)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": a})
}

// apiAccountRename renames a linked account: {label}.
func (s *Server) apiAccountRename(w http.ResponseWriter, r *http.Request) {
	id, ok := accountID(r)
	if !ok {
		writeAPIError(w, http.StatusBadRequest, "bad id")
		return
	}
	var b struct {
		Label string `json:"label"`
	}
	if err := decodeBody(r, &b); err != nil || strings.TrimSpace(b.Label) == "" {
		writeAPIError(w, http.StatusBadRequest, "a label is required")
		return
	}
	if err := s.accts.Rename(r.Context(), id, strings.TrimSpace(b.Label)); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// apiAccountRemove unlinks an account (not the active one).
func (s *Server) apiAccountRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := accountID(r)
	if !ok {
		writeAPIError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.accts.Remove(r.Context(), id); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- encryption keys -------------------------------------------------------------

// apiEncryptionKeysAdd imports decryption keys (key backup files, multipart
// field "keys") and returns the Connections view.
func (s *Server) apiEncryptionKeysAdd(w http.ResponseWriter, r *http.Request) {
	if s.crypt == nil {
		writeAPIError(w, http.StatusNotFound, "encryption unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // keys are tiny
	mr, err := r.MultipartReader()
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "send the key files as multipart form data")
		return
	}
	added, known, rejected := 0, 0, 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "couldn't read the upload: "+err.Error())
			return
		}
		if part.FormName() != "keys" || part.FileName() == "" {
			_, _ = io.Copy(io.Discard, part)
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(part, 8192))
		name, aerr := s.crypt.AddKeyRaw(raw)
		switch {
		case aerr != nil:
			rejected++
			s.log.Warn("encryption key import rejected", "file", part.FileName(), "err", aerr)
		case name == "":
			known++
		default:
			added++
			s.log.Info("encryption key added", "as", name)
		}
	}
	if added == 0 && known == 0 {
		writeAPIError(w, http.StatusBadRequest, "That isn't a gpix key backup.")
		return
	}
	var notice string
	switch {
	case added > 0 && rejected > 0:
		notice = fmt.Sprintf("Added %d key(s); %d file(s) weren't key backups.", added, rejected)
	case added > 0:
		notice = fmt.Sprintf("Added %d key(s).", added)
	default:
		notice = "That key is already loaded."
	}
	writeJSON(w, http.StatusOK, s.gatewaysView(r, "", notice))
}

// ---- video streaming ----------------------------------------------------------------

// apiMediaStream returns signed URLs for playing an item: an HLS playlist
// (adaptive quality; also opens in VLC) and the original file.
func (s *Server) apiMediaStream(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeAPIError(w, http.StatusBadRequest, "missing key")
		return
	}
	ttl := time.Duration(s.cfg.StreamTokenTTLMin) * time.Minute
	tok := s.signMedia(key, ttl)
	writeJSON(w, http.StatusOK, map[string]any{
		"hls":         "/stream/" + tok,
		"hlsAbsolute": s.baseURL(r) + "/stream/" + tok,
		"raw":         "/raw/" + tok + "?inline=1",
		"expiresAt":   time.Now().Add(ttl).UTC(),
	})
}

// ---- public share page ------------------------------------------------------------------

type publicShareItem struct {
	Name    string `json:"name"`
	IsVideo bool   `json:"isVideo"`
	Thumb   string `json:"thumb"`
	Raw     string `json:"raw"`
}

type publicShare struct {
	// State is "ok", "password" (unlock first), "expired" or "missing".
	State         string            `json:"state"`
	Title         string            `json:"title,omitempty"`
	AllowOriginal bool              `json:"allowOriginal"`
	Download      string            `json:"download,omitempty"`
	Items         []publicShareItem `json:"items"`
}

func (s *Server) publicShareView(sh share.Share, authorized bool) publicShare {
	if !authorized {
		return publicShare{State: "password", Items: []publicShareItem{}}
	}
	v := publicShare{State: "ok", Title: sh.Title, AllowOriginal: sh.AllowOriginal, Items: []publicShareItem{}}
	base := "/s/" + sh.Token
	for i, it := range sh.Items {
		idx := strconv.Itoa(i)
		v.Items = append(v.Items, publicShareItem{
			Name:    it.FileName,
			IsVideo: it.IsVideo,
			Thumb:   base + "/thumb/" + idx,
			Raw:     base + "/raw/" + idx,
		})
	}
	if sh.AllowOriginal && len(sh.Items) > 0 {
		v.Download = base + "/download"
	}
	return v
}

// apiShareInfo serves GET /s/{token}/info for the web app's share page.
func (s *Server) apiShareInfo(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		writeJSON(w, http.StatusOK, publicShare{State: "missing", Items: []publicShareItem{}})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	sh, err := s.share.Get(r.Context(), r.PathValue("token"))
	if err != nil {
		writeJSON(w, http.StatusOK, publicShare{State: "missing", Items: []publicShareItem{}})
		return
	}
	if !sh.Active(time.Now()) {
		writeJSON(w, http.StatusOK, publicShare{State: "expired", Title: sh.Title, Items: []publicShareItem{}})
		return
	}
	writeJSON(w, http.StatusOK, s.publicShareView(sh, s.shareAuthorized(r, sh)))
}

// apiShareUnlock serves POST /s/{token}/unlock {password}: on success it sets
// the access cookie (scoped to /s/{token}) and returns the share.
func (s *Server) apiShareUnlock(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		writeAPIError(w, http.StatusNotFound, "This link doesn't exist.")
		return
	}
	sh, err := s.share.Get(r.Context(), r.PathValue("token"))
	if err != nil || !sh.Active(time.Now()) {
		writeAPIError(w, http.StatusNotFound, "This link has expired or doesn't exist.")
		return
	}
	var b struct {
		Password string `json:"password"`
	}
	if err := decodeBody(r, &b); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json")
		return
	}
	if !sh.VerifyPassword(b.Password) {
		time.Sleep(300 * time.Millisecond) // dampen guessing
		writeAPIError(w, http.StatusForbidden, "Incorrect password.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     shareCookieName(sh.Token),
		Value:    s.signShareAccess(sh.Token, 12*time.Hour),
		Path:     "/s/" + sh.Token,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((12 * time.Hour).Seconds()),
	})
	writeJSON(w, http.StatusOK, s.publicShareView(sh, true))
}
