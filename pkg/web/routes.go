package web

import (
	"io/fs"
	"net/http"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("POST /logout", s.requireSession(s.handleLogout))

	mux.HandleFunc("GET /auth/logto/login", s.handleLogtoLogin)
	mux.HandleFunc("GET /auth/logto/callback", s.handleLogtoCallback)

	mux.HandleFunc("GET /{$}", s.requireSession(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/browse", http.StatusSeeOther)
	}))
	mux.HandleFunc("GET /browse", s.requireSession(s.handleBrowse))
	mux.HandleFunc("GET /view/{key}", s.requireSession(s.handleView))
	mux.HandleFunc("GET /thumb/{key}", s.requireSession(s.handleThumb))
	mux.HandleFunc("GET /upload", s.requireSession(s.handleUploadForm))
	mux.HandleFunc("POST /upload", s.requireSession(s.handleUploadSubmit))
	mux.HandleFunc("POST /delete/{key}", s.requireSession(s.handleDelete))
	mux.HandleFunc("GET /api/upload-progress/{id}", s.requireSession(s.handleProgressSSE))

	mux.HandleFunc("GET /settings/import", s.requireSession(s.handleImportPage))
	mux.HandleFunc("POST /settings/import/start", s.requireSession(s.handleImportStart))
	mux.HandleFunc("GET /settings/import/status", s.requireSession(s.handleImportStatus))
	mux.HandleFunc("POST /settings/import/stop", s.requireSession(s.handleImportStop))

	mux.HandleFunc("GET /settings/api-tokens", s.requireSession(s.handleAPITokensPage))
	mux.HandleFunc("POST /settings/api-tokens/create", s.requireSession(s.handleAPITokenCreate))
	mux.HandleFunc("POST /settings/api-tokens/revoke/{id}", s.requireSession(s.handleAPITokenRevoke))

	mux.HandleFunc("GET /settings/gateways", s.requireSession(s.handleGateways))
	mux.HandleFunc("POST /settings/gateways/regenerate", s.requireSession(s.handleGatewaysRegenerate))
	mux.HandleFunc("POST /settings/gateways/clear", s.requireSession(s.handleGatewaysClear))
	mux.HandleFunc("POST /settings/gateways/encryption", s.requireSession(s.handleEncryptionToggle))
	mux.HandleFunc("GET /settings/gateways/encryption-key", s.requireSession(s.handleEncryptionKeyBackup))

	mux.HandleFunc("GET /stream/{token}", s.handleStream)
	mux.HandleFunc("GET /raw/{token}", s.handleRaw)

	// Share management (authenticated).
	mux.HandleFunc("POST /share/create", s.requireSession(s.handleShareCreate))
	mux.HandleFunc("GET /settings/shares", s.requireSession(s.handleSharesList))
	mux.HandleFunc("POST /share/revoke/{token}", s.requireSession(s.handleShareRevoke))

	// Public share links (no session).
	mux.HandleFunc("GET /s/{token}", s.handleSharePage)
	mux.HandleFunc("POST /s/{token}", s.handleSharePassword)
	mux.HandleFunc("GET /s/{token}/thumb/{idx}", s.handleShareThumb)
	mux.HandleFunc("GET /s/{token}/raw/{idx}", s.handleShareRaw)

	// JSON API for native clients (Bearer API-token auth). See pkg/web/api.go.
	mux.HandleFunc("GET /api/v1/ping", s.requireAPIToken(s.apiPing))
	mux.HandleFunc("GET /api/v1/media", s.requireAPIToken(s.apiMediaList))
	mux.HandleFunc("GET /api/v1/media/{key}/thumb", s.requireAPIToken(s.apiThumb))
	mux.HandleFunc("GET /api/v1/media/{key}/original", s.requireAPIToken(s.apiOriginal))
	mux.HandleFunc("DELETE /api/v1/media/{key}", s.requireAPIToken(s.apiDelete))
	mux.HandleFunc("POST /api/v1/upload", s.requireAPIToken(s.apiUpload))
	mux.HandleFunc("GET /api/v1/shares", s.requireAPIToken(s.apiSharesList))
	mux.HandleFunc("POST /api/v1/shares", s.requireAPIToken(s.apiShareCreate))
	mux.HandleFunc("DELETE /api/v1/shares/{token}", s.requireAPIToken(s.apiShareRevoke))
	mux.HandleFunc("GET /api/v1/import/status", s.requireAPIToken(s.apiImportStatus))
	mux.HandleFunc("POST /api/v1/import/start", s.requireAPIToken(s.apiImportStart))
	mux.HandleFunc("POST /api/v1/import/stop", s.requireAPIToken(s.apiImportStop))

	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(sub)))

	return s.recoverPanic(s.logRequest(s.securityHeaders(mux)))
}
