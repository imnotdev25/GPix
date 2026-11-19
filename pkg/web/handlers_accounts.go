package web

import (
	"net/http"
	"strconv"
)

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
