package web

import (
	"context"
	"net/http"
	"strings"
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

// renderLogin renders the legacy login page (still used for single sign-on
// errors).
func (s *Server) renderLogin(w http.ResponseWriter, errMsg string) {
	s.render(w, "login", pageData{
		Error:         errMsg,
		LogtoEnabled:  s.cfg.LogtoEnabled(),
		PasswordLogin: s.cfg.PasswordLoginEnabled(),
	})
}

// handleLoginSubmit keeps the legacy form POST (fields u, p) working.
func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.PasswordLoginEnabled() || s.users == nil {
		s.renderLogin(w, "Password sign-in is disabled. Use single sign-on.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderLogin(w, "bad form")
		return
	}
	time.Sleep(250 * time.Millisecond) // dampen brute force
	u, err := s.users.Authenticate(r.Context(), strings.TrimSpace(r.FormValue("u")), r.FormValue("p"))
	if err != nil {
		s.renderLogin(w, "Wrong email or password.")
		return
	}
	s.setSessionCookie(w, u.Email, time.Duration(s.cfg.SessionDays)*24*time.Hour)
	http.Redirect(w, r, "/library", http.StatusSeeOther)
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

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
