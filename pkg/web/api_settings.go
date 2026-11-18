package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"gpix/pkg/users"
)

// ---- auth (SPA) ------------------------------------------------------------

// accountCount returns how many sign-in accounts exist (0 if unknown).
func (s *Server) accountCount(r *http.Request) int {
	if s.users == nil {
		return 0
	}
	n, err := s.users.Count(r.Context())
	if err != nil {
		return 0
	}
	return n
}

// signupOpen reports whether a new password account can be created right now:
// password sign-in is on and the GPIX_MAX_USERS cap isn't reached.
func (s *Server) signupOpen(r *http.Request) bool {
	if s.users == nil || !s.cfg.PasswordLoginEnabled() {
		return false
	}
	return s.cfg.MaxUsers == 0 || s.accountCount(r) < s.cfg.MaxUsers
}

// apiAuthMe reports whether the caller is signed in and which sign-in options
// the server offers (used by the SPA to pick sign-in, sign-up or SSO).
func (s *Server) apiAuthMe(w http.ResponseWriter, r *http.Request) {
	user := ""
	if c, err := r.Cookie(sessionCookieName); err == nil {
		if u, verr := s.verifySession(c.Value); verr == nil {
			user = u
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":          user,
		"signedIn":      user != "",
		"passwordLogin": s.cfg.PasswordLoginEnabled(),
		"logtoEnabled":  s.cfg.LogtoEnabled(),
		"signupOpen":    s.signupOpen(r),
		"firstRun":      s.users != nil && s.accountCount(r) == 0,
	})
}

// apiAuthLogin signs in with email + password and sets the session cookie.
// "username" is accepted too (older clients, imported legacy logins).
func (s *Server) apiAuthLogin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.PasswordLoginEnabled() {
		writeAPIError(w, http.StatusForbidden, "password sign-in is disabled; use single sign-on")
		return
	}
	if s.users == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "accounts unavailable")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json")
		return
	}
	time.Sleep(250 * time.Millisecond) // dampen brute force
	u, err := s.users.Authenticate(r.Context(), strings.TrimSpace(firstNonEmpty(body.Email, body.Username)), body.Password)
	if errors.Is(err, users.ErrBadPassword) {
		writeAPIError(w, http.StatusUnauthorized, "wrong email or password")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setSessionCookie(w, u.Email, time.Duration(s.cfg.SessionDays)*24*time.Hour)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": u.Email})
}

// apiAuthSignup creates an email + password account and signs it in. Sign-up
// is open until GPIX_MAX_USERS accounts exist (1 by default), and only for
// emails on GPIX_SIGNUP_ALLOWLIST when that's set.
func (s *Server) apiAuthSignup(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.PasswordLoginEnabled() || s.users == nil {
		writeAPIError(w, http.StatusForbidden, "sign-up is disabled on this server")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
		writeAPIError(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	if len(body.Password) < 8 {
		writeAPIError(w, http.StatusBadRequest, "use a password of at least 8 characters")
		return
	}
	if len(body.Password) > 72 {
		writeAPIError(w, http.StatusBadRequest, "use a shorter password (72 bytes at most)")
		return
	}
	if !users.Allowlisted(email, s.cfg.SignupAllowlist) {
		writeAPIError(w, http.StatusForbidden, "this email isn't allowed to sign up on this server")
		return
	}
	hash, err := users.HashPassword(body.Password)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	err = s.users.Register(r.Context(), users.User{
		Subject: "local:" + email, Email: email, Name: email, PasswordHash: hash,
	}, s.cfg.MaxUsers)
	switch {
	case errors.Is(err, users.ErrClosed):
		writeAPIError(w, http.StatusForbidden, "sign-up is closed: this server already has its maximum number of accounts")
		return
	case errors.Is(err, users.ErrExists):
		writeAPIError(w, http.StatusConflict, "an account with this email already exists; sign in instead")
		return
	case err != nil:
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.log.Info("account created", "email", email)
	s.setSessionCookie(w, email, time.Duration(s.cfg.SessionDays)*24*time.Hour)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": email})
}

// apiAuthLogout clears the session cookie.
func (s *Server) apiAuthLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- gateways / encryption -------------------------------------------------

func (s *Server) apiGateways(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.gatewaysView(r, "", ""))
}

func (s *Server) apiGatewaysRegenerate(w http.ResponseWriter, r *http.Request) {
	if s.gw == nil {
		writeAPIError(w, http.StatusNotFound, "gateways unavailable")
		return
	}
	var body struct {
		Kind string `json:"kind"` // "s3" | "webdav"
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
	var err error
	switch body.Kind {
	case "s3":
		_, _, err = s.gw.RegenerateS3()
	case "webdav":
		_, err = s.gw.RegenerateWebDAV()
	default:
		writeAPIError(w, http.StatusBadRequest, `kind must be "s3" or "webdav"`)
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.gatewaysView(r, body.Kind, "New credentials generated — copy them now."))
}

func (s *Server) apiGatewaysClear(w http.ResponseWriter, r *http.Request) {
	if s.gw == nil {
		writeAPIError(w, http.StatusNotFound, "gateways unavailable")
		return
	}
	var body struct {
		Kind string `json:"kind"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
	var err error
	switch body.Kind {
	case "s3":
		err = s.gw.ClearS3()
	case "webdav":
		err = s.gw.ClearWebDAV()
	default:
		writeAPIError(w, http.StatusBadRequest, `kind must be "s3" or "webdav"`)
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.gatewaysView(r, "", "Credentials cleared."))
}

func (s *Server) apiEncryptionToggle(w http.ResponseWriter, r *http.Request) {
	if s.crypt == nil {
		writeAPIError(w, http.StatusNotFound, "encryption unavailable")
		return
	}
	var body struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json")
		return
	}
	if err := s.crypt.SetEnabled(body.Enable); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": body.Enable})
}

// ---- API tokens ------------------------------------------------------------

func (s *Server) apiTokensList(w http.ResponseWriter, r *http.Request) {
	if s.apitokens == nil {
		writeAPIError(w, http.StatusNotFound, "api tokens unavailable")
		return
	}
	list, err := s.apitokens.List(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": list})
}

func (s *Server) apiTokensCreate(w http.ResponseWriter, r *http.Request) {
	if s.apitokens == nil {
		writeAPIError(w, http.StatusNotFound, "api tokens unavailable")
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
	secret, tok, err := s.apitokens.Create(r.Context(), body.Label)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The plaintext is returned exactly once.
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "secret": secret})
}

func (s *Server) apiTokensRevoke(w http.ResponseWriter, r *http.Request) {
	if s.apitokens == nil {
		writeAPIError(w, http.StatusNotFound, "api tokens unavailable")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.apitokens.Revoke(r.Context(), id); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- danger zone -----------------------------------------------------------

// apiDeleteAll trashes the whole library. The SPA collects three escalating
// confirmations plus a typed phrase; the server re-checks the phrase.
func (s *Server) apiDeleteAll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json")
		return
	}
	if body.Confirm != "DELETE" {
		writeAPIError(w, http.StatusBadRequest, `confirmation phrase must be "DELETE"`)
		return
	}
	lib := s.libc()
	if lib == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "library unavailable")
		return
	}
	ctx, cancel := withTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	items, err := lib.All(ctx)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	keys := make([]string, 0, len(items))
	for _, it := range items {
		keys = append(keys, it.MediaKey)
	}
	const batch = 100
	failed := 0
	for i := 0; i < len(keys); i += batch {
		if ctx.Err() != nil {
			break
		}
		end := i + batch
		if end > len(keys) {
			end = len(keys)
		}
		res, derr := s.gpc().DeleteByMediaKeys(ctx, keys[i:end], false)
		if derr != nil {
			failed += end - i
			continue
		}
		for _, e := range res {
			if e != nil {
				failed++
			}
		}
	}
	lib.Invalidate()
	s.log.Warn("delete-all executed", "user", userFromCtx(r.Context()), "total", len(keys), "failed", failed)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "total": len(keys), "failed": failed})
}
