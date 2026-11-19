package web

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"gpix/pkg/oidc"
	"gpix/pkg/users"
)

const oauthCookieName = "gpix_oauth"

func (s *Server) loginError(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/signin?error="+url.QueryEscape(msg), http.StatusSeeOther)
}

// handleLogtoLogin starts the OIDC authorization-code + PKCE flow.
func (s *Server) handleLogtoLogin(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.NotFound(w, r)
		return
	}
	verifier, challenge := oidc.PKCE()
	state := oidc.RandomState()
	nonce := oidc.RandomState()

	ctx, cancel := withTimeout(r.Context(), 15*time.Second)
	defer cancel()
	authURL, err := s.oidc.AuthCodeURL(ctx, state, nonce, challenge)
	if err != nil {
		s.log.Error("logto auth url", "err", err)
		s.loginError(w, r, "Could not reach the login provider.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthCookieName,
		Value:    s.signOAuthState(oauthState{State: state, Nonce: nonce, Verifier: verifier}, 10*time.Minute),
		Path:     "/auth/logto",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

// handleLogtoCallback completes the OIDC flow: validates state, exchanges the
// code, fetches identity, applies the allowlist + max-users policy, and starts
// a session.
func (s *Server) handleLogtoCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.NotFound(w, r)
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		s.loginError(w, r, "Sign-in was cancelled or failed.")
		return
	}
	c, err := r.Cookie(oauthCookieName)
	if err != nil {
		s.loginError(w, r, "Your sign-in session expired. Please try again.")
		return
	}
	st, ok := s.verifyOAuthState(c.Value)
	if !ok || r.URL.Query().Get("state") != st.State {
		s.loginError(w, r, "Invalid sign-in request. Please try again.")
		return
	}
	// Clear the one-time oauth cookie.
	http.SetCookie(w, &http.Cookie{Name: oauthCookieName, Value: "", Path: "/auth/logto", MaxAge: -1})

	ctx, cancel := withTimeout(r.Context(), 20*time.Second)
	defer cancel()

	tokens, err := s.oidc.Exchange(ctx, r.URL.Query().Get("code"), st.Verifier)
	if err != nil {
		s.log.Error("logto exchange", "err", err)
		s.loginError(w, r, "Could not complete sign-in.")
		return
	}
	ui, err := s.oidc.UserInfo(ctx, tokens.AccessToken)
	if err != nil {
		s.log.Error("logto userinfo", "err", err)
		s.loginError(w, r, "Could not read your profile.")
		return
	}

	email := ui.Email
	name := firstNonEmpty(ui.Name, ui.Username, email, ui.Subject)

	// First-time sign-ins follow the same allowlist and account cap as
	// password sign-up.
	if s.users != nil {
		if _, gerr := s.users.Get(ctx, ui.Subject); errors.Is(gerr, users.ErrNotFound) {
			if !users.Allowlisted(email, s.cfg.SignupAllowlist) {
				s.loginError(w, r, "This account is not on the allowlist.")
				return
			}
			rerr := s.users.Register(ctx, users.User{Subject: ui.Subject, Email: email, Name: name}, s.cfg.MaxUsers)
			switch {
			case errors.Is(rerr, users.ErrClosed):
				s.loginError(w, r, "Sign-up is closed: this server already has its maximum number of accounts.")
				return
			case rerr != nil && !errors.Is(rerr, users.ErrExists):
				s.log.Error("logto create user", "err", rerr)
				s.loginError(w, r, "Could not create your account.")
				return
			}
		} else if gerr != nil {
			s.log.Error("logto user lookup", "err", gerr)
			s.loginError(w, r, "Could not read your account.")
			return
		}
	}

	sessUser := email
	if sessUser == "" {
		sessUser = ui.Subject
	}
	s.setSessionCookie(w, sessUser, time.Duration(s.cfg.SessionDays)*24*time.Hour)
	http.Redirect(w, r, "/library", http.StatusSeeOther)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
