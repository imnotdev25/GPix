package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ---- auth (SPA) ------------------------------------------------------------

// apiAuthMe reports whether the caller is signed in and which auth options the
// server offers (used by the SPA to decide between the login form and SSO).
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
	})
}

// apiAuthLogin signs in with username + password and sets the session cookie.
func (s *Server) apiAuthLogin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.PasswordLoginEnabled() {
		writeAPIError(w, http.StatusForbidden, "password login is disabled; use single sign-on")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json")
		return
	}
	time.Sleep(250 * time.Millisecond) // dampen brute force
	if body.Username != s.cfg.Username ||
		bcrypt.CompareHashAndPassword([]byte(s.cfg.PasswordHash), []byte(body.Password)) != nil {
		writeAPIError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	s.setSessionCookie(w, body.Username, time.Duration(s.cfg.SessionDays)*24*time.Hour)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": body.Username})
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
