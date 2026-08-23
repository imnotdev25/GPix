package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// The built React SPA (see ui/). `all:` includes files starting with "_" and
// "." that Vite may emit. The directory ships with a placeholder index.html so
// the Go build works before anyone runs `npm run build`.
//
//go:embed all:dist
var spaFS embed.FS

// spaHandler serves the SPA: hashed assets with a long cache, and index.html
// for every other path so client-side routing works on deep links/refresh.
func (s *Server) spaHandler() http.Handler {
	sub, err := fs.Sub(spaFS, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "ui not built", http.StatusInternalServerError)
		})
	}
	files := http.FileServer(http.FS(sub))

	index, ierr := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" && p != "." {
			if f, err := sub.Open(p); err == nil {
				f.Close()
				// Vite emits content-hashed filenames under assets/.
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		if ierr != nil {
			http.Error(w, "ui not built — run `npm ci && npm run build` in ui/", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
