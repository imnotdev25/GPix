package web

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleAPITokensPage(w http.ResponseWriter, r *http.Request) {
	if s.apitokens == nil {
		http.Error(w, "API tokens unavailable", http.StatusNotFound)
		return
	}
	tokens, _ := s.apitokens.List(r.Context())
	s.render(w, "apitokens", pageData{
		User:      userFromCtx(r.Context()),
		Title:     "API tokens",
		APITokens: tokens,
	})
}

func (s *Server) handleAPITokenCreate(w http.ResponseWriter, r *http.Request) {
	if s.apitokens == nil {
		http.Error(w, "API tokens unavailable", http.StatusNotFound)
		return
	}
	_ = r.ParseForm()
	label := strings.TrimSpace(r.FormValue("label"))
	secret, _, err := s.apitokens.Create(r.Context(), label)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tokens, _ := s.apitokens.List(r.Context())
	// Render the same page with the plaintext shown once (not via redirect, so it
	// never lands in a URL or the access log).
	s.render(w, "apitokens", pageData{
		User:      userFromCtx(r.Context()),
		Title:     "API tokens",
		APITokens: tokens,
		NewToken:  secret,
	})
}

func (s *Server) handleAPITokenRevoke(w http.ResponseWriter, r *http.Request) {
	if s.apitokens == nil {
		http.Error(w, "API tokens unavailable", http.StatusNotFound)
		return
	}
	if id, err := strconv.ParseInt(r.PathValue("id"), 10, 64); err == nil {
		_ = s.apitokens.Revoke(r.Context(), id)
	}
	http.Redirect(w, r, "/settings/api-tokens", http.StatusSeeOther)
}
