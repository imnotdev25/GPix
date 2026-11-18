package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"gpix/pkg/gpmc"
)

const maxBulkDeleteKeys = 5000

// apiMediaBulkDelete moves several items to the Google Photos trash in one
// request. Keys known to the library cache are addressed by the SHA-1 it
// already holds (no extra library walk); unknown keys fall back to a lookup.
//
//	POST /api/v1/media/delete  {"keys": ["...", ...]}
//	→ {"deleted": ["..."], "failed": 2}
func (s *Server) apiMediaBulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json")
		return
	}
	if len(body.Keys) == 0 {
		writeAPIError(w, http.StatusBadRequest, "no keys")
		return
	}
	if len(body.Keys) > maxBulkDeleteKeys {
		writeAPIError(w, http.StatusBadRequest, "too many keys in one request")
		return
	}
	rt := s.accts.Active()
	if rt == nil || rt.Lib == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "library unavailable")
		return
	}

	ctx, cancel := withTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	seen := make(map[string]bool, len(body.Keys))
	var known []gpmc.MediaItem
	var unknown []string
	for _, k := range body.Keys {
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		if it, ok := rt.Lib.Peek(k); ok && len(it.SHA1) > 0 {
			known = append(known, it)
		} else {
			unknown = append(unknown, k)
		}
	}

	deleted, failed := trashMediaItems(ctx, rt.GP, known)
	if len(unknown) > 0 {
		res, err := rt.GP.DeleteByMediaKeys(ctx, unknown, false)
		if err != nil {
			failed += len(unknown)
		} else {
			for _, k := range unknown {
				if e, ok := res[k]; ok && e == nil {
					deleted = append(deleted, k)
				} else {
					failed++
				}
			}
		}
	}

	if s.uploads != nil && len(deleted) > 0 {
		if err := s.uploads.Forget(context.Background(), rt.ID, deleted); err != nil {
			s.log.Warn("upload log: forget deleted items", "err", err)
		}
	}
	rt.Lib.Remove(deleted...)
	if deleted == nil {
		deleted = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "failed": failed})
}
