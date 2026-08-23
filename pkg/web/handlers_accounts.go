package web

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleAccountsPage(w http.ResponseWriter, r *http.Request) {
	list, err := s.accts.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	view := make([]accountView, 0, len(list))
	for _, a := range list {
		view = append(view, accountView{
			ID:      a.ID,
			Label:   a.Label,
			Active:  a.Active,
			AddedAt: a.AddedAt,
		})
	}
	s.render(w, "accounts", pageData{
		User:     userFromCtx(r.Context()),
		Title:    "Accounts",
		Accounts: view,
		Notice:   r.URL.Query().Get("notice"),
		Error:    r.URL.Query().Get("err"),
	})
}

func (s *Server) handleAccountAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	label := strings.TrimSpace(r.FormValue("label"))
	authData := strings.TrimSpace(r.FormValue("auth_data"))
	if authData == "" {
		http.Redirect(w, r, "/settings/accounts?err="+urlEscape("auth_data is required"), http.StatusSeeOther)
		return
	}
	if _, err := s.accts.Add(r.Context(), label, authData); err != nil {
		http.Redirect(w, r, "/settings/accounts?err="+urlEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings/accounts?notice="+urlEscape("Account linked."), http.StatusSeeOther)
}

func (s *Server) handleAccountSwitch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.accts.SetActive(r.Context(), id); err != nil {
		http.Redirect(w, r, "/settings/accounts?err="+urlEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/browse", http.StatusSeeOther)
}

func (s *Server) handleAccountRemove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.accts.Remove(r.Context(), id); err != nil {
		http.Redirect(w, r, "/settings/accounts?err="+urlEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings/accounts?notice="+urlEscape("Account unlinked."), http.StatusSeeOther)
}

func (s *Server) handleAccountRename(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	_ = r.ParseForm()
	if err := s.accts.Rename(r.Context(), id, r.FormValue("label")); err != nil {
		http.Redirect(w, r, "/settings/accounts?err="+urlEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings/accounts", http.StatusSeeOther)
}

// apiAccounts lists linked accounts for the app/SPA.
func (s *Server) apiAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := s.accts.List(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": list, "activeId": s.accts.ActiveID()})
}

// apiAccountSwitch switches the active account.
func (s *Server) apiAccountSwitch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.accts.SetActive(r.Context(), id); err != nil {
		writeAPIError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "activeId": id})
}

func urlEscape(s string) string {
	return strings.NewReplacer(" ", "+", "&", "%26", "?", "%3F", "#", "%23").Replace(s)
}
