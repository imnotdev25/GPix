package web

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

// buildEndpoint renders a user-facing URL for a listen address. Wildcard binds
// (":9000", "0.0.0.0:9000", "[::]:9000") are not browsable, so it substitutes
// the hostname the browser actually used to reach gpix.
func buildEndpoint(listen string, r *http.Request) string {
	if listen == "" {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return scheme + "://" + listen
	}
	switch host {
	case "", "0.0.0.0", "::", "::0":
		reqHost := r.Host
		if h, _, e := net.SplitHostPort(reqHost); e == nil {
			reqHost = h
		}
		host = reqHost
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

// webdavUser is the WebDAV username to show: the signed-in account's email.
func webdavUser(r *http.Request) string {
	if u := userFromCtx(r.Context()); !strings.HasPrefix(u, "api:") {
		return u
	}
	return "" // signed in with an app token, not an account
}

func (s *Server) gatewaysView(r *http.Request, justGenerated, notice string) *gatewaysView {
	access, secret := "", ""
	davPass := ""
	if s.gw != nil {
		access, secret = s.gw.S3()
		davPass = s.gw.WebDAVPassword()
	}
	encAvailable := s.crypt != nil
	encEnabled := false
	encFingerprint := ""
	var encKeys []gwKeyInfo
	if encAvailable {
		encEnabled = s.crypt.Enabled()
		encFingerprint = s.crypt.Fingerprint()
		for _, k := range s.crypt.Keys() {
			encKeys = append(encKeys, gwKeyInfo{Fingerprint: k.Fingerprint, Active: k.Active})
		}
	}
	return &gatewaysView{
		S3Enabled:   s.cfg.S3Listen != "",
		S3Endpoint:  buildEndpoint(s.cfg.S3Listen, r),
		S3Region:    s.cfg.S3Region,
		S3Bucket:    s.cfg.S3Bucket,
		S3AccessKey: access,
		S3SecretKey: secret,
		HasS3Keys:   access != "" && secret != "",

		WebDAVEnabled:  s.cfg.WebDAVListen != "",
		WebDAVEndpoint: buildEndpoint(s.cfg.WebDAVListen, r),
		WebDAVUsername: webdavUser(r),
		WebDAVPassword: davPass,
		HasWebDAVPass:  davPass != "",

		EncryptionAvailable:   encAvailable,
		EncryptionEnabled:     encEnabled,
		EncryptionFingerprint: encFingerprint,
		EncryptionKeyCount:    len(encKeys),
		EncryptionKeys:        encKeys,

		JustGenerated: justGenerated,
		Notice:        notice,
	}
}

func (s *Server) handleEncryptionKeyBackup(w http.ResponseWriter, r *http.Request) {
	if s.crypt == nil {
		http.Error(w, "encryption unavailable", http.StatusNotFound)
		return
	}
	key := s.crypt.BackupBytes()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="gpix-encryption.key"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(key)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(key)
}

// gatewaysView models the Connections settings page.
type gatewaysView struct {
	S3Enabled   bool
	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	HasS3Keys   bool

	WebDAVEnabled  bool
	WebDAVEndpoint string
	WebDAVUsername string
	WebDAVPassword string
	HasWebDAVPass  bool

	EncryptionAvailable   bool
	EncryptionEnabled     bool
	EncryptionFingerprint string
	EncryptionKeyCount    int
	EncryptionKeys        []gwKeyInfo

	// JustGenerated is "s3" or "webdav" right after a regenerate, so the page
	// can reveal the new secret with a "save this now" prompt. Empty otherwise.
	JustGenerated string
	Notice        string
}

// gwKeyInfo is a loaded encryption key shown on the Connections page.
type gwKeyInfo struct {
	Fingerprint string
	Active      bool
}
