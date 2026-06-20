package web

import (
	"net/http"
	"strconv"
	"time"

	"gpix/pkg/share"
)

// baseURL returns the externally-visible base URL: the configured ServerURL if
// set, otherwise derived from the current request.
func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.ServerURL != "" {
		return s.cfg.ServerURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func shareCookieName(token string) string { return "gpix_share_" + token }

// --- authenticated management ---

func (s *Server) handleShareCreate(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		http.Error(w, "sharing not enabled", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	key := r.FormValue("key")
	if key == "" {
		http.Error(w, "missing media key", http.StatusBadRequest)
		return
	}
	var ttl time.Duration
	if h := r.FormValue("expiry_hours"); h != "" {
		if n, err := strconv.Atoi(h); err == nil && n > 0 {
			ttl = time.Duration(n) * time.Hour
		}
	}
	var maxDl int64
	if m := r.FormValue("max_downloads"); m != "" {
		if n, err := strconv.ParseInt(m, 10, 64); err == nil && n > 0 {
			maxDl = n
		}
	}
	_, err := s.share.Create(r.Context(), share.CreateParams{
		MediaKey:      key,
		FileName:      r.FormValue("filename"),
		IsVideo:       r.FormValue("is_video") == "1",
		Password:      r.FormValue("password"),
		TTL:           ttl,
		MaxDownloads:  maxDl,
		AllowOriginal: r.FormValue("allow_original") != "0",
	})
	if err != nil {
		s.log.Error("share create", "err", err)
		http.Error(w, "could not create share: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/settings/shares", http.StatusSeeOther)
}

func (s *Server) handleSharesList(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		http.Error(w, "sharing not enabled", http.StatusNotFound)
		return
	}
	shares, err := s.share.List(r.Context())
	if err != nil {
		s.render(w, "error", pageData{User: userFromCtx(r.Context()), Title: "shares", Message: err.Error()})
		return
	}
	base := s.baseURL(r)
	items := make([]shareItem, 0, len(shares))
	for _, sh := range shares {
		items = append(items, shareItem{
			Token:        sh.Token,
			URL:          base + "/s/" + sh.Token,
			FileName:     sh.FileName,
			IsVideo:      sh.IsVideo,
			HasPassword:  sh.HasPassword,
			ExpiresLabel: expiresLabel(sh.ExpiresAt),
			Downloads:    sh.Downloads,
			MaxDownloads: sh.MaxDownloads,
			CreatedAt:    sh.CreatedAt,
		})
	}
	s.render(w, "shares", pageData{
		User:   userFromCtx(r.Context()),
		Title:  "Shares",
		Shares: items,
	})
}

func (s *Server) handleShareRevoke(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		http.Error(w, "sharing not enabled", http.StatusNotFound)
		return
	}
	token := r.PathValue("token")
	if token != "" {
		_ = s.share.Delete(r.Context(), token)
	}
	http.Redirect(w, r, "/settings/shares", http.StatusSeeOther)
}

func expiresLabel(t time.Time) string {
	if t.IsZero() {
		return "Never"
	}
	if time.Now().After(t) {
		return "Expired"
	}
	return t.Format("Jan 2, 2006 15:04")
}

// --- public share endpoints (no session) ---

// shareAuthorized reports whether the request may view the share: shares without
// a password are always open; password-protected shares require a valid cookie.
func (s *Server) shareAuthorized(r *http.Request, sh share.Share) bool {
	if !sh.HasPassword {
		return true
	}
	c, err := r.Cookie(shareCookieName(sh.Token))
	if err != nil {
		return false
	}
	return s.verifyShareAccess(sh.Token, c.Value)
}

func (s *Server) loadShare(w http.ResponseWriter, r *http.Request) (share.Share, bool) {
	token := r.PathValue("token")
	sh, err := s.share.Get(r.Context(), token)
	if err != nil {
		s.render(w, "share_public", pageData{SharePublic: &sharePublicView{Expired: true, Error: "This link does not exist."}})
		return share.Share{}, false
	}
	if !sh.Active(time.Now()) {
		s.render(w, "share_public", pageData{SharePublic: &sharePublicView{Token: sh.Token, FileName: sh.FileName, Expired: true}})
		return share.Share{}, false
	}
	return sh, true
}

func (s *Server) handleSharePage(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		http.NotFound(w, r)
		return
	}
	sh, ok := s.loadShare(w, r)
	if !ok {
		return
	}
	view := &sharePublicView{
		Token:         sh.Token,
		FileName:      sh.FileName,
		IsVideo:       sh.IsVideo,
		AllowOriginal: sh.AllowOriginal,
	}
	if !s.shareAuthorized(r, sh) {
		view.NeedsPassword = true
	}
	s.render(w, "share_public", pageData{SharePublic: view})
}

func (s *Server) handleSharePassword(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		http.NotFound(w, r)
		return
	}
	sh, ok := s.loadShare(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !sh.VerifyPassword(r.FormValue("password")) {
		s.render(w, "share_public", pageData{SharePublic: &sharePublicView{
			Token: sh.Token, FileName: sh.FileName, IsVideo: sh.IsVideo,
			NeedsPassword: true, Error: "Incorrect password.",
		}})
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
	http.Redirect(w, r, "/s/"+sh.Token, http.StatusSeeOther)
}

func (s *Server) handleShareThumb(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		http.NotFound(w, r)
		return
	}
	sh, err := s.share.Get(r.Context(), r.PathValue("token"))
	if err != nil || !sh.Active(time.Now()) || !s.shareAuthorized(r, sh) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.serveThumb(w, r, sh.MediaKey, thumbSize(r))
}

func (s *Server) handleShareRaw(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		http.NotFound(w, r)
		return
	}
	sh, err := s.share.Get(r.Context(), r.PathValue("token"))
	if err != nil || !sh.Active(time.Now()) || !s.shareAuthorized(r, sh) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !sh.AllowOriginal {
		http.Error(w, "download disabled for this share", http.StatusForbidden)
		return
	}
	_ = s.share.RecordDownload(r.Context(), sh.Token)
	s.proxyDownload(w, r, sh.MediaKey, true)
}
