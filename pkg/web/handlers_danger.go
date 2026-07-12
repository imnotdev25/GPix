package web

import (
	"net/http"
	"time"
)

// handleDeleteAll moves every item in the library to the Google Photos trash.
// It is intentionally destructive and only proceeds when the form carries the
// exact confirmation phrase the UI collects behind three escalating dialogs.
func (s *Server) handleDeleteAll(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if r.FormValue("confirm") != "DELETE" {
		// Missing/typo'd confirmation — do nothing.
		http.Redirect(w, r, "/settings/gateways", http.StatusSeeOther)
		return
	}
	if s.lib == nil {
		http.Error(w, "library unavailable", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := withTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	items, err := s.lib.All(ctx)
	if err != nil {
		s.render(w, "error", pageData{
			User:    userFromCtx(r.Context()),
			Title:   "Delete all failed",
			Message: err.Error(),
		})
		return
	}
	keys := make([]string, 0, len(items))
	for _, it := range items {
		keys = append(keys, it.MediaKey)
	}

	// Delete in batches so one huge request doesn't stall or time out.
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
		res, derr := s.gp.DeleteByMediaKeys(ctx, keys[i:end], false) // false = trash, not permanent
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
	s.lib.Invalidate()
	s.log.Warn("delete-all executed", "user", userFromCtx(r.Context()), "total", len(keys), "failed", failed)
	http.Redirect(w, r, "/browse", http.StatusSeeOther)
}
