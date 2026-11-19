package web

import (
	"io/fs"
	"net/http"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Every page is the React web app (registered last, on "/"). The server
	// only serves the JSON API, media, share links and single sign-on.

	// Sign-in lives in the web app; old bookmarks land there.
	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("GET /auth/logto/login", s.handleLogtoLogin)
	mux.HandleFunc("GET /auth/logto/callback", s.handleLogtoCallback)

	// Addresses of the old server-rendered pages, sent to their new home.
	redirect := func(to string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, to, http.StatusFound) }
	}
	for from, to := range map[string]string{
		"GET /browse":              "/library",
		"GET /view/{key}":          "/library",
		"GET /settings/gateways":   "/connections",
		"GET /settings/accounts":   "/accounts",
		"GET /settings/api-tokens": "/tokens",
		"GET /settings/import":     "/import",
		"GET /settings/shares":     "/shares",
	} {
		mux.HandleFunc(from, redirect(to))
	}

	// Signed media links: HLS streams (also for VLC) and originals.
	mux.HandleFunc("GET /stream/{token}", s.handleStream)
	mux.HandleFunc("GET /raw/{token}", s.handleRaw)

	// Public share links (no session). GET /s/{token} itself is the web app's
	// share page; these are what it loads.
	mux.HandleFunc("GET /s/{token}/info", s.apiShareInfo)
	mux.HandleFunc("POST /s/{token}/unlock", s.apiShareUnlock)
	mux.HandleFunc("GET /s/{token}/thumb/{idx}", s.handleShareThumb)
	mux.HandleFunc("GET /s/{token}/raw/{idx}", s.handleShareRaw)
	mux.HandleFunc("GET /s/{token}/download", s.handleShareDownload)

	// JSON API: the web app (session cookie) and native clients (Bearer token).
	mux.HandleFunc("GET /api/v1/ping", s.requireAPIToken(s.apiPing))
	mux.HandleFunc("GET /api/v1/media", s.requireAPIToken(s.apiMediaList))
	mux.HandleFunc("GET /api/v1/media/{key}/thumb", s.requireAPIToken(s.apiThumb))
	mux.HandleFunc("GET /api/v1/media/{key}/original", s.requireAPIToken(s.apiOriginal))
	mux.HandleFunc("GET /api/v1/media/{key}/stream", s.requireAPIToken(s.apiMediaStream))
	mux.HandleFunc("DELETE /api/v1/media/{key}", s.requireAPIToken(s.apiDelete))
	mux.HandleFunc("POST /api/v1/media/delete", s.requireAPIToken(s.apiMediaBulkDelete))
	mux.HandleFunc("GET /api/v1/media/timeline", s.requireAPIToken(s.apiMediaTimeline))
	mux.HandleFunc("POST /api/v1/upload", s.requireAPIToken(s.apiUpload))

	mux.HandleFunc("GET /api/v1/shares", s.requireAPIToken(s.apiSharesList))
	mux.HandleFunc("POST /api/v1/shares", s.requireAPIToken(s.apiShareCreate))
	mux.HandleFunc("DELETE /api/v1/shares/{token}", s.requireAPIToken(s.apiShareRevoke))

	// Sign-in for the web app (session cookie).
	mux.HandleFunc("GET /api/v1/auth/me", s.apiAuthMe)
	mux.HandleFunc("POST /api/v1/auth/login", s.apiAuthLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.apiAuthLogout)
	mux.HandleFunc("POST /api/v1/auth/signup", s.apiAuthSignup)

	mux.HandleFunc("GET /api/v1/gateways", s.requireAPIToken(s.apiGateways))
	mux.HandleFunc("POST /api/v1/gateways/regenerate", s.requireAPIToken(s.apiGatewaysRegenerate))
	mux.HandleFunc("POST /api/v1/gateways/clear", s.requireAPIToken(s.apiGatewaysClear))
	mux.HandleFunc("POST /api/v1/encryption/toggle", s.requireAPIToken(s.apiEncryptionToggle))
	mux.HandleFunc("GET /api/v1/encryption/key-backup", s.requireAPIToken(s.handleEncryptionKeyBackup))
	mux.HandleFunc("POST /api/v1/encryption/keys", s.requireAPIToken(s.apiEncryptionKeysAdd))

	mux.HandleFunc("GET /api/v1/tokens", s.requireAPIToken(s.apiTokensList))
	mux.HandleFunc("POST /api/v1/tokens", s.requireAPIToken(s.apiTokensCreate))
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", s.requireAPIToken(s.apiTokensRevoke))

	mux.HandleFunc("POST /api/v1/delete-all", s.requireAPIToken(s.apiDeleteAll))
	mux.HandleFunc("GET /api/v1/gpix-uploads/summary", s.requireAPIToken(s.apiGpixUploadsSummary))
	mux.HandleFunc("POST /api/v1/gpix-uploads/delete", s.requireAPIToken(s.apiGpixUploadsDelete))

	// gpix's own albums and tags (not synced with Google Photos).
	mux.HandleFunc("GET /api/v1/albums", s.requireAPIToken(s.apiAlbumsList))
	mux.HandleFunc("POST /api/v1/albums", s.requireAPIToken(s.apiAlbumCreate))
	mux.HandleFunc("GET /api/v1/albums/{id}", s.requireAPIToken(s.apiAlbumGet))
	mux.HandleFunc("POST /api/v1/albums/{id}", s.requireAPIToken(s.apiAlbumRename))
	mux.HandleFunc("DELETE /api/v1/albums/{id}", s.requireAPIToken(s.apiAlbumDelete))
	mux.HandleFunc("POST /api/v1/albums/{id}/items", s.requireAPIToken(s.apiAlbumAddItems))
	mux.HandleFunc("POST /api/v1/albums/{id}/items/remove", s.requireAPIToken(s.apiAlbumRemoveItems))
	mux.HandleFunc("GET /api/v1/tags", s.requireAPIToken(s.apiTagsList))
	mux.HandleFunc("POST /api/v1/tags/add", s.requireAPIToken(s.apiTagsAdd))
	mux.HandleFunc("POST /api/v1/tags/remove", s.requireAPIToken(s.apiTagsRemove))

	mux.HandleFunc("GET /api/v1/accounts", s.requireAPIToken(s.apiAccounts))
	mux.HandleFunc("POST /api/v1/accounts", s.requireAPIToken(s.apiAccountAdd))
	mux.HandleFunc("POST /api/v1/accounts/{id}", s.requireAPIToken(s.apiAccountRename))
	mux.HandleFunc("DELETE /api/v1/accounts/{id}", s.requireAPIToken(s.apiAccountRemove))
	mux.HandleFunc("POST /api/v1/accounts/{id}/switch", s.requireAPIToken(s.apiAccountSwitch))

	mux.HandleFunc("GET /api/v1/import/status", s.requireAPIToken(s.apiImportStatus))
	mux.HandleFunc("POST /api/v1/import/start", s.requireAPIToken(s.apiImportStart))
	mux.HandleFunc("POST /api/v1/import/google-start", s.requireAPIToken(s.apiImportGoogleStart))
	mux.HandleFunc("POST /api/v1/import/stop", s.requireAPIToken(s.apiImportStop))

	// hls.js for the web app's video player.
	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(sub)))

	// The web app handles every other path (its routing is client-side).
	mux.Handle("GET /", s.spaHandler())

	// logRequest is outermost so a recovered panic is still logged as a 500.
	return s.logRequest(s.recoverPanic(s.securityHeaders(s.corsAPI(mux))))
}
