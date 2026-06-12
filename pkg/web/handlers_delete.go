package web

import (
	"net/http"
	"time"
)

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}
	permanent := r.URL.Query().Get("permanent") == "1"

	ctx, cancel := withTimeout(r.Context(), 30*time.Second)
	defer cancel()

	results, err := s.gp.DeleteByMediaKeys(ctx, []string{key}, permanent)
	if err != nil {
		s.log.Error("delete", "key", key, "err", err)
		http.Error(w, "delete failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if e := results[key]; e != nil {
		s.log.Error("delete item", "key", key, "err", e)
		http.Error(w, "delete failed: "+e.Error(), http.StatusBadGateway)
		return
	}
	s.log.Info("deleted", "key", key, "permanent", permanent, "user", userFromCtx(r.Context()))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if permanent {
		_, _ = w.Write([]byte("deleted permanently"))
	} else {
		_, _ = w.Write([]byte("moved to trash"))
	}
}
