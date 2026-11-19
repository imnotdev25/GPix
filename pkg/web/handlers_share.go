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

// --- public share endpoints (no session) ---

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

// shareItemByIndex loads a share and resolves one item, enforcing
// active + authorized access.
func (s *Server) shareItemByIndex(r *http.Request) (share.Share, share.ShareItem, bool) {
	sh, err := s.share.Get(r.Context(), r.PathValue("token"))
	if err != nil || !sh.Active(time.Now()) || !s.shareAuthorized(r, sh) {
		return share.Share{}, share.ShareItem{}, false
	}
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil || idx < 0 || idx >= len(sh.Items) {
		return share.Share{}, share.ShareItem{}, false
	}
	return sh, sh.Items[idx], true
}

func (s *Server) handleShareThumb(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		http.NotFound(w, r)
		return
	}
	_, item, ok := s.shareItemByIndex(r)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.serveThumb(w, r, item.MediaKey, thumbSize(r))
}

func (s *Server) handleShareRaw(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		http.NotFound(w, r)
		return
	}
	sh, item, ok := s.shareItemByIndex(r)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !sh.AllowOriginal && r.URL.Query().Get("inline") != "1" {
		http.Error(w, "download disabled for this share", http.StatusForbidden)
		return
	}
	if r.URL.Query().Get("inline") != "1" {
		if err := s.share.RecordDownload(r.Context(), sh.Token); err != nil {
			s.log.Warn("share: count download", "share", redactPath("/s/"+sh.Token), "err", err)
		}
	}
	attachment := r.URL.Query().Get("inline") != "1"
	s.proxyDownload(w, r, item.MediaKey, attachment)
}
