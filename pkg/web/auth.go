package web

import (
	"context"
	"net/http"
	"time"
)

const sessionCookieName = "gpix_session"

type ctxKey int

const ctxKeyUser ctxKey = 1

func userFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyUser).(string)
	return v
}

// handleLoginForm: the web app owns sign-in at /signin now, so the old
// server-rendered page just redirects there.
func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		if _, verr := s.verifySession(c.Value); verr == nil {
			http.Redirect(w, r, "/library", http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/signin", http.StatusSeeOther)
}

// setSessionCookie issues the signed session cookie. It uses SameSite=Lax (not
// Strict) so the cookie is sent on the top-level redirect that completes an
// external OIDC login — with Strict the browser withholds it on that
// cross-site-initiated navigation and the user bounces back to sign-in. Lax
// still withholds the cookie from cross-site POSTs, so state-changing endpoints
// stay CSRF-safe.
func (s *Server) setSessionCookie(w http.ResponseWriter, username string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    s.signSession(username, ttl),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}
