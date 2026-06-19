package web

import (
	"net"
	"net/http"
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

func (s *Server) gatewaysView(r *http.Request, justGenerated, notice string) *gatewaysView {
	access, secret := "", ""
	davPass := ""
	if s.gw != nil {
		access, secret = s.gw.S3()
		davPass = s.gw.WebDAVPassword()
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
		WebDAVUsername: s.cfg.Username,
		WebDAVPassword: davPass,
		HasWebDAVPass:  davPass != "",

		JustGenerated: justGenerated,
		Notice:        notice,
	}
}

func (s *Server) handleGateways(w http.ResponseWriter, r *http.Request) {
	s.render(w, "gateways", pageData{
		User:     userFromCtx(r.Context()),
		Title:    "Connections",
		Gateways: s.gatewaysView(r, "", ""),
	})
}

func (s *Server) handleGatewaysRegenerate(w http.ResponseWriter, r *http.Request) {
	if s.gw == nil {
		http.Error(w, "gateway credentials unavailable", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	which := r.FormValue("which")
	var notice string
	switch which {
	case "s3":
		if _, _, err := s.gw.RegenerateS3(); err != nil {
			s.renderGatewayError(w, r, "Could not save S3 keys: "+err.Error())
			return
		}
		notice = "New S3 keys generated. Copy the secret now — it is shown below."
	case "webdav":
		if _, err := s.gw.RegenerateWebDAV(); err != nil {
			s.renderGatewayError(w, r, "Could not save WebDAV password: "+err.Error())
			return
		}
		notice = "New WebDAV app password generated. Copy it now."
	default:
		http.Error(w, "unknown target", http.StatusBadRequest)
		return
	}
	s.render(w, "gateways", pageData{
		User:     userFromCtx(r.Context()),
		Title:    "Connections",
		Gateways: s.gatewaysView(r, which, notice),
	})
}

func (s *Server) handleGatewaysClear(w http.ResponseWriter, r *http.Request) {
	if s.gw == nil {
		http.Error(w, "gateway credentials unavailable", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	var notice string
	switch r.FormValue("which") {
	case "s3":
		_ = s.gw.ClearS3()
		notice = "S3 keys cleared. The S3 endpoint will reject all requests until you generate new keys."
	case "webdav":
		_ = s.gw.ClearWebDAV()
		notice = "WebDAV app password cleared. The main login password still works for WebDAV."
	default:
		http.Error(w, "unknown target", http.StatusBadRequest)
		return
	}
	s.render(w, "gateways", pageData{
		User:     userFromCtx(r.Context()),
		Title:    "Connections",
		Gateways: s.gatewaysView(r, "", notice),
	})
}

func (s *Server) renderGatewayError(w http.ResponseWriter, r *http.Request, msg string) {
	s.render(w, "gateways", pageData{
		User:     userFromCtx(r.Context()),
		Title:    "Connections",
		Error:    msg,
		Gateways: s.gatewaysView(r, "", ""),
	})
}
