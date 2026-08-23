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

	// "/" is served by the React SPA (registered at the bottom). The legacy
	// server-rendered pages remain available at their own paths.
	mux.HandleFunc("GET /browse", s.requireSession(s.handleBrowse))
	mux.HandleFunc("GET /view/{key}", s.requireSession(s.handleView))
	mux.HandleFunc("GET /thumb/{key}", s.requireSession(s.handleThumb))
	mux.HandleFunc("GET /upload", s.requireSession(s.handleUploadForm))
	mux.HandleFunc("POST /upload", s.requireSession(s.handleUploadSubmit))
	mux.HandleFunc("POST /delete/{key}", s.requireSession(s.handleDelete))
	mux.HandleFunc("GET /api/upload-progress/{id}", s.requireSession(s.handleProgressSSE))

	mux.HandleFunc("GET /settings/import", s.requireSession(s.handleImportPage))
	mux.HandleFunc("POST /settings/import/start", s.requireSession(s.handleImportStart))
	mux.HandleFunc("POST /settings/import/google-start", s.requireSession(s.handleImportGoogleStart))
	mux.HandleFunc("GET /settings/import/status", s.requireSession(s.handleImportStatus))
	mux.HandleFunc("POST /settings/import/stop", s.requireSession(s.handleImportStop))

	mux.HandleFunc("GET /settings/accounts", s.requireSession(s.handleAccountsPage))
	mux.HandleFunc("POST /settings/accounts/add", s.requireSession(s.handleAccountAdd))
	mux.HandleFunc("POST /settings/accounts/{id}/switch", s.requireSession(s.handleAccountSwitch))
	mux.HandleFunc("POST /settings/accounts/{id}/remove", s.requireSession(s.handleAccountRemove))
	mux.HandleFunc("POST /settings/accounts/{id}/rename", s.requireSession(s.handleAccountRename))

	mux.HandleFunc("GET /settings/api-tokens", s.requireSession(s.handleAPITokensPage))
	mux.HandleFunc("POST /settings/api-tokens/create", s.requireSession(s.handleAPITokenCreate))
	mux.HandleFunc("POST /settings/api-tokens/revoke/{id}", s.requireSession(s.handleAPITokenRevoke))

	mux.HandleFunc("POST /settings/delete-all", s.requireSession(s.handleDeleteAll))
	mux.HandleFunc("GET /settings/gateways", s.requireSession(s.handleGateways))
	mux.HandleFunc("POST /settings/gateways/regenerate", s.requireSession(s.handleGatewaysRegenerate))
	mux.HandleFunc("POST /settings/gateways/clear", s.requireSession(s.handleGatewaysClear))
	mux.HandleFunc("POST /settings/gateways/encryption", s.requireSession(s.handleEncryptionToggle))
	mux.HandleFunc("GET /settings/gateways/encryption-key", s.requireSession(s.handleEncryptionKeyBackup))
	mux.HandleFunc("POST /settings/gateways/encryption-keys", s.requireSession(s.handleEncryptionKeyUpload))

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
	// Auth for the SPA (session-cookie based; no token needed to check/login).
	mux.HandleFunc("GET /api/v1/auth/me", s.apiAuthMe)
	mux.HandleFunc("POST /api/v1/auth/login", s.apiAuthLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.apiAuthLogout)

	mux.HandleFunc("GET /api/v1/gateways", s.requireAPIToken(s.apiGateways))
	mux.HandleFunc("POST /api/v1/gateways/regenerate", s.requireAPIToken(s.apiGatewaysRegenerate))
	mux.HandleFunc("POST /api/v1/gateways/clear", s.requireAPIToken(s.apiGatewaysClear))
	mux.HandleFunc("POST /api/v1/encryption/toggle", s.requireAPIToken(s.apiEncryptionToggle))

	mux.HandleFunc("GET /api/v1/tokens", s.requireAPIToken(s.apiTokensList))
	mux.HandleFunc("POST /api/v1/tokens", s.requireAPIToken(s.apiTokensCreate))
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", s.requireAPIToken(s.apiTokensRevoke))

	mux.HandleFunc("POST /api/v1/delete-all", s.requireAPIToken(s.apiDeleteAll))

	mux.HandleFunc("GET /api/v1/accounts", s.requireAPIToken(s.apiAccounts))
	mux.HandleFunc("POST /api/v1/accounts/{id}/switch", s.requireAPIToken(s.apiAccountSwitch))
	mux.HandleFunc("GET /api/v1/import/status", s.requireAPIToken(s.apiImportStatus))
	mux.HandleFunc("POST /api/v1/import/start", s.requireAPIToken(s.apiImportStart))
	mux.HandleFunc("POST /api/v1/import/google-start", s.requireAPIToken(s.apiImportGoogleStart))
	mux.HandleFunc("POST /api/v1/import/stop", s.requireAPIToken(s.apiImportStop))

	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(sub)))

	// The React SPA handles everything not matched above (its own routing lives
	// client-side). Registered on "/" so the more specific patterns win; the
	// legacy server-rendered pages stay reachable at their existing paths.
	mux.Handle("GET /", s.spaHandler())

	return s.recoverPanic(s.logRequest(s.securityHeaders(s.corsAPI(mux))))
}
