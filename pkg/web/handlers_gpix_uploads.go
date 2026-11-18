package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gpix/pkg/accounts"
	"gpix/pkg/gpmc"
)

// "Delete gpix uploads" removes only what gpix put in the library: items in the
// local upload log (pkg/uploads) plus anything gpix wrapped as an MP4 (non-media
// files and encrypted uploads, recognisable by filename). Plain photos gpix
// uploaded before the log existed can't be told apart from the user's own and
// are left alone.

const gpixUploadSampleSize = 8

type gpixUploadSummary struct {
	AccountID    int64  `json:"accountId"`
	AccountLabel string `json:"accountLabel"`

	Total  int   `json:"total"`
	Photos int   `json:"photos"`
	Videos int   `json:"videos"`
	Docs   int   `json:"docs"`
	Files  int   `json:"files"`
	Bytes  int64 `json:"bytes"`

	// Tracked items matched via the upload log; Disguised items matched only
	// because gpix wrapped them.
	Tracked   int `json:"tracked"`
	Disguised int `json:"disguised"`

	TrackingEnabled bool      `json:"trackingEnabled"`
	TrackingSince   time.Time `json:"trackingSince"`

	Sample []string `json:"sample"` // a few display names, for the confirmation
}

// selectGpixUploads picks the items gpix uploaded out of a library listing.
func selectGpixUploads(items []gpmc.MediaItem, tracked map[string]bool) ([]gpmc.MediaItem, gpixUploadSummary) {
	var sel []gpmc.MediaItem
	sum := gpixUploadSummary{Sample: []string{}}
	for _, it := range items {
		display, class, disguised := classifyItem(it.Filename, it.Kind)
		isTracked := tracked[it.MediaKey]
		if !isTracked && !disguised {
			continue
		}
		sel = append(sel, it)
		if isTracked {
			sum.Tracked++
		} else {
			sum.Disguised++
		}
		switch class {
		case classPhoto:
			sum.Photos++
		case classVideo:
			sum.Videos++
		case classDoc:
			sum.Docs++
		default:
			sum.Files++
		}
		sum.Bytes += it.SizeBytes
		if len(sum.Sample) < gpixUploadSampleSize {
			sum.Sample = append(sum.Sample, display)
		}
	}
	sum.Total = len(sel)
	return sel, sum
}

// gpixUploads lists the active account's gpix uploads. fresh forces a full
// re-listing from Google instead of using the cached one.
func (s *Server) gpixUploads(ctx context.Context, rt *accounts.Runtime, fresh bool) ([]gpmc.MediaItem, gpixUploadSummary, error) {
	if fresh {
		if err := rt.Lib.Refresh(ctx); err != nil {
			return nil, gpixUploadSummary{}, fmt.Errorf("list library: %w", err)
		}
	}
	items, err := rt.Lib.All(ctx)
	if err != nil {
		return nil, gpixUploadSummary{}, fmt.Errorf("list library: %w", err)
	}
	tracked := map[string]bool{}
	if s.uploads != nil {
		if tracked, err = s.uploads.Keys(ctx, rt.ID); err != nil {
			return nil, gpixUploadSummary{}, fmt.Errorf("upload log: %w", err)
		}
	}
	sel, sum := selectGpixUploads(items, tracked)
	sum.AccountID = rt.ID
	sum.AccountLabel = rt.Label
	if s.uploads != nil {
		sum.TrackingEnabled = true
		sum.TrackingSince = s.uploads.Since()
	}
	return sel, sum, nil
}

// apiGpixUploadsSummary reports what "delete gpix uploads" would remove.
func (s *Server) apiGpixUploadsSummary(w http.ResponseWriter, r *http.Request) {
	rt := s.accts.Active()
	if rt == nil || rt.Lib == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "library unavailable")
		return
	}
	ctx, cancel := withTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	_, sum, err := s.gpixUploads(ctx, rt, false)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

// apiGpixUploadsDelete moves every gpix upload to the Google Photos trash. The
// client echoes the account and count it showed the user; if a fresh listing no
// longer matches, nothing is deleted and the client must review again.
func (s *Server) apiGpixUploadsDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm   string `json:"confirm"`
		AccountID int64  `json:"accountId"`
		Expected  int    `json:"expected"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json")
		return
	}
	if body.Confirm != "DELETE" {
		writeAPIError(w, http.StatusBadRequest, `confirmation phrase must be "DELETE"`)
		return
	}
	rt := s.accts.Active()
	if rt == nil || rt.Lib == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "library unavailable")
		return
	}
	if body.AccountID != rt.ID {
		writeAPIError(w, http.StatusConflict, "the active account changed since you reviewed; review again")
		return
	}

	ctx, cancel := withTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	sel, sum, err := s.gpixUploads(ctx, rt, true)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	if sum.Total != body.Expected {
		writeAPIError(w, http.StatusConflict, fmt.Sprintf(
			"your library changed since you reviewed it: %d item(s) now match, not %d. Nothing was deleted; review again.",
			sum.Total, body.Expected))
		return
	}

	deleted, failed := trashMediaItems(ctx, rt.GP, sel)
	if s.uploads != nil {
		if ferr := s.uploads.Forget(context.Background(), rt.ID, deleted); ferr != nil {
			s.log.Warn("upload log: forget deleted items", "err", ferr)
		}
	}
	rt.Lib.Remove(deleted...)
	s.log.Warn("delete-gpix-uploads executed",
		"user", userFromCtx(r.Context()), "account", rt.Label,
		"total", sum.Total, "deleted", len(deleted), "failed", failed)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "total": sum.Total, "deleted": len(deleted), "failed": failed,
	})
}

// trashMediaItems moves items to the trash in batches, addressing them by the
// SHA-1 dedup key the listing already carries (no extra library walk). Items
// without a hash can't be addressed and count as failed.
func trashMediaItems(ctx context.Context, gp *gpmc.Client, items []gpmc.MediaItem) (deleted []string, failed int) {
	type target struct{ key, dedup string }
	targets := make([]target, 0, len(items))
	for _, it := range items {
		if dk := it.DedupKey(); dk != "" {
			targets = append(targets, target{it.MediaKey, dk})
		} else {
			failed++
		}
	}
	const batch = 100
	for i := 0; i < len(targets); i += batch {
		end := min(i+batch, len(targets))
		if ctx.Err() != nil {
			failed += len(targets) - i
			break
		}
		dks := make([]string, 0, end-i)
		for _, t := range targets[i:end] {
			dks = append(dks, t.dedup)
		}
		if err := gp.TrashItems(ctx, dks); err != nil {
			failed += end - i
			continue
		}
		for _, t := range targets[i:end] {
			deleted = append(deleted, t.key)
		}
	}
	return deleted, failed
}
