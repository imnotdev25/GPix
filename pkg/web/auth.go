package web

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "gpix_session"

type ctxKey int

const ctxKeyUser ctxKey = 1

func userFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyUser).(string)
	return v
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(sessionCookieName); err == nil {
		if c, _ := r.Cookie(sessionCookieName); c != nil {
			if _, err := s.verifySession(c.Value); err == nil {
				http.Redirect(w, r, "/browse", http.StatusSeeOther)
				return
			}
		}
	}
	s.renderLogin(w, "")
}

// renderLogin renders the login page with the current auth options.
func (s *Server) renderLogin(w http.ResponseWriter, errMsg string) {
	s.render(w, "login", pageData{
		Error:         errMsg,
		LogtoEnabled:  s.cfg.LogtoEnabled(),
		PasswordLogin: s.cfg.PasswordLoginEnabled(),
	})
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.PasswordLoginEnabled() {
		s.renderLogin(w, "Password login is disabled. Use the single sign-on option.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderLogin(w, "bad form")
		return
	}
	u := r.FormValue("u")
	p := r.FormValue("p")

	dampDelay := 300 * time.Millisecond
	if u != s.cfg.Username {
		time.Sleep(dampDelay)
		s.renderLogin(w, "invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(s.cfg.PasswordHash), []byte(p)); err != nil {
		time.Sleep(dampDelay)
		s.renderLogin(w, "invalid credentials")
		return
	}

	ttl := time.Duration(s.cfg.SessionDays) * 24 * time.Hour
	s.setSessionCookie(w, u, ttl)
	http.Redirect(w, r, "/browse", http.StatusSeeOther)
}

// setSessionCookie issues the signed session cookie. It uses SameSite=Lax (not
// Strict) so the cookie is sent on the top-level redirect that completes an
// external OIDC login — with Strict the browser withholds it on that
// cross-site-initiated navigation and the user bounces back to /login. Lax still
// withholds the cookie from cross-site POSTs, so state-changing endpoints stay
// CSRF-safe.
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
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
