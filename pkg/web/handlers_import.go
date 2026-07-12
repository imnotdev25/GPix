package web

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func (s *Server) handleImportPage(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		http.Error(w, "import not available", http.StatusNotFound)
		return
	}
	s.render(w, "import", pageData{User: userFromCtx(r.Context()), Title: "Import"})
}

func (s *Server) handleImportStart(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		http.Error(w, "import not available", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	serverURL := r.FormValue("server_url")
	apiKey := r.FormValue("api_key")
	if serverURL == "" || apiKey == "" {
		http.Error(w, "Immich server URL and API key are required", http.StatusBadRequest)
		return
	}
	workers := 3
	if n, err := strconv.Atoi(r.FormValue("workers")); err == nil && n > 0 {
		workers = n
	}
	if err := s.importer.Start(serverURL, apiKey, workers); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/settings/import", http.StatusSeeOther)
}

func (s *Server) handleImportGoogleStart(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		http.Error(w, "import not available", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	authData := r.FormValue("auth_data")
	if authData == "" {
		http.Error(w, "source GP_AUTH_DATA is required", http.StatusBadRequest)
		return
	}
	workers := 3
	if n, err := strconv.Atoi(r.FormValue("workers")); err == nil && n > 0 {
		workers = n
	}
	if err := s.importer.StartGoogle(authData, workers); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/settings/import", http.StatusSeeOther)
}

func (s *Server) handleImportStatus(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		http.Error(w, "import not available", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.importer.Status())
}

func (s *Server) handleImportStop(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		http.Error(w, "import not available", http.StatusNotFound)
		return
	}
	s.importer.Stop()
	http.Redirect(w, r, "/settings/import", http.StatusSeeOther)
}
