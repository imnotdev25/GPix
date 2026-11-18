package web

import (
	"archive/zip"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"gpix/pkg/share"
)

// directURL is a share's direct download link, or "" when it can't have one:
// a password-protected share needs its page first, and a share without
// "allow originals" can't be downloaded at all.
func (s *Server) directURL(sh share.Share) string {
	if sh.HasPassword || !sh.AllowOriginal {
		return ""
	}
	return s.shareURL(sh.Token) + "/download"
}

// handleShareDownload serves GET /s/{token}/download: the file itself for a
// one-item share, or a .zip of every item. It needs the same access as the
// share page and counts as one download.
func (s *Server) handleShareDownload(w http.ResponseWriter, r *http.Request) {
	if s.share == nil {
		http.NotFound(w, r)
		return
	}
	sh, err := s.share.Get(r.Context(), r.PathValue("token"))
	if err != nil || !sh.Active(time.Now()) {
		http.Error(w, "This link has expired or doesn't exist.", http.StatusNotFound)
		return
	}
	if !s.shareAuthorized(r, sh) {
		http.Redirect(w, r, "/s/"+sh.Token, http.StatusSeeOther) // the password page
		return
	}
	if !sh.AllowOriginal || len(sh.Items) == 0 {
		http.Error(w, "Downloads are turned off for this link.", http.StatusForbidden)
		return
	}
	if err := s.share.RecordDownload(r.Context(), sh.Token); err != nil {
		s.log.Warn("share: count download", "share", redactPath("/s/"+sh.Token), "err", err)
	}
	if len(sh.Items) == 1 {
		s.proxyDownload(w, r, sh.Items[0].MediaKey, true)
		return
	}

	name := strings.TrimSpace(sh.Title)
	if name == "" {
		name = "gpix-share"
	}
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": name + ".zip"})
	if cd == "" {
		cd = `attachment; filename="gpix-share.zip"`
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", cd)
	zw := zip.NewWriter(w)
	used := map[string]int{}
	for _, it := range sh.Items {
		ctx, cancel := withTimeout(r.Context(), 10*time.Minute)
		rc, _, oerr := s.openDecrypted(ctx, it.MediaKey)
		if oerr != nil {
			cancel()
			s.log.Warn("share zip: open item", "err", oerr)
			continue
		}
		// Photos and videos are already compressed: store them as they are.
		fw, cerr := zw.CreateHeader(&zip.FileHeader{Name: zipName(used, it.FileName), Method: zip.Store, Modified: time.Now()})
		if cerr == nil {
			_, cerr = io.Copy(fw, rc)
		}
		rc.Close()
		cancel()
		if cerr != nil {
			s.log.Warn("share zip: write item", "err", cerr)
			break // most likely the client went away
		}
	}
	_ = zw.Close()
}

// zipName makes a safe, unique file name for a zip entry.
func zipName(used map[string]int, name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = "file"
	}
	n := used[name]
	used[name] = n + 1
	if n == 0 {
		return name
	}
	ext := path.Ext(name)
	return strings.TrimSuffix(name, ext) + " (" + strconv.Itoa(n+1) + ")" + ext
}
