package web

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // IANA zones for ?tz=, even on minimal images without zoneinfo

	"gpix/pkg/accounts"
	"gpix/pkg/albums"
	"gpix/pkg/gpmc"
)

// Pages are small so the first screen of photos arrives quickly.
const (
	pageDefault = 50
	pageMax     = 50
)

// A cursor names the last item of a page (its time and key), so paging stays
// right while items are added or removed in between.
func encodeCursor(it gpmc.MediaItem) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(it.Mtime.UnixMilli(), 10) + ":" + it.MediaKey))
}

func decodeCursor(c string) (ms int64, key string, ok bool) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0, "", false
	}
	msStr, key, found := strings.Cut(string(b), ":")
	if !found || key == "" {
		return 0, "", false
	}
	ms, err = strconv.ParseInt(msStr, 10, 64)
	return ms, key, err == nil
}

// cursorIndex finds a cursor's item in a newest-first listing. If the item is
// gone, it returns where it would be (the first item not newer than it).
func cursorIndex(items []gpmc.MediaItem, ms int64, key string) (int, bool) {
	i := sort.Search(len(items), func(i int) bool { return items[i].Mtime.UnixMilli() <= ms })
	for j := i; j < len(items) && items[j].Mtime.UnixMilli() == ms; j++ {
		if items[j].MediaKey == key {
			return j, true
		}
	}
	return i, false
}

type listErr struct {
	code int
	msg  string
}

// listingFor returns the active account's listing, newest first, narrowed to
// an album (?album=<id>) or a tag (?tag=<name>) when asked.
func (s *Server) listingFor(ctx context.Context, rt *accounts.Runtime, r *http.Request) ([]gpmc.MediaItem, *listErr) {
	all, err := rt.Lib.All(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, &listErr{http.StatusServiceUnavailable, "your library is still loading for the first time; try again in a moment"}
	}
	if err != nil {
		return nil, &listErr{http.StatusBadGateway, err.Error()}
	}
	q := r.URL.Query()
	var keep map[string]bool
	switch {
	case q.Get("album") != "":
		if s.albums == nil {
			return nil, &listErr{http.StatusNotFound, "albums unavailable"}
		}
		id, perr := strconv.ParseInt(q.Get("album"), 10, 64)
		if perr != nil {
			return nil, &listErr{http.StatusBadRequest, "bad album id"}
		}
		keep, err = s.albums.Keys(ctx, rt.ID, id)
		if errors.Is(err, albums.ErrNotFound) {
			return nil, &listErr{http.StatusNotFound, "no such album"}
		}
	case q.Get("tag") != "":
		if s.albums == nil {
			return nil, &listErr{http.StatusNotFound, "tags unavailable"}
		}
		keep, err = s.albums.TagKeys(ctx, rt.ID, albums.NormalizeTag(q.Get("tag")))
	default:
		return all, nil
	}
	if err != nil {
		return nil, &listErr{http.StatusInternalServerError, err.Error()}
	}
	out := make([]gpmc.MediaItem, 0, len(keep))
	for _, it := range all {
		if keep[it.MediaKey] {
			out = append(out, it)
		}
	}
	return out, nil
}

// apiMediaList returns a page of the library, newest first, from gpix's
// listing cache: at most 50 items, so the first screen loads fast.
//
//	?limit=N          page size (1–50, default 50)
//	?cursor=C         the page after C (the previous page's nextCursor)
//	?before=C         the page before C (prevCursor), for scrolling back up
//	?from=MS          start at the first item taken before MS (ms since the
//	                  epoch), to jump to a month on the timeline
//	?album=ID, ?tag=T only that album's or tag's items
//
// Response: {items, nextCursor, prevCursor, offset, total}. offset is the
// first item's position in the whole (filtered) listing; a cursor is empty
// when there's nothing more that way.
func (s *Server) apiMediaList(w http.ResponseWriter, r *http.Request) {
	rt := s.accts.Active()
	if rt == nil || rt.Lib == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "library unavailable")
		return
	}
	ctx, cancel := withTimeout(r.Context(), 25*time.Second)
	defer cancel()
	src, lerr := s.listingFor(ctx, rt, r)
	if lerr != nil {
		writeAPIError(w, lerr.code, lerr.msg)
		return
	}

	q := r.URL.Query()
	limit := pageDefault
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		limit = min(n, pageMax)
	}
	start, end := 0, -1
	switch c, b := q.Get("cursor"), q.Get("before"); {
	case b != "":
		end = len(src)
		if ms, key, ok := decodeCursor(b); ok {
			end, _ = cursorIndex(src, ms, key)
		}
		start = max(0, end-limit)
	case q.Get("from") != "":
		ms, _ := strconv.ParseInt(q.Get("from"), 10, 64)
		start = sort.Search(len(src), func(i int) bool { return src[i].Mtime.UnixMilli() < ms })
	case c != "" && allDigits(c):
		start, _ = strconv.Atoi(c) // older app builds sent an offset
	case c != "":
		start = len(src) // a cursor this version can't read: nothing more
		if ms, key, ok := decodeCursor(c); ok {
			i, found := cursorIndex(src, ms, key)
			if found {
				i++
			}
			start = i
		}
	}
	start = min(max(start, 0), len(src))
	if end < 0 {
		end = min(start+limit, len(src))
	}

	page := src[start:end]
	var tags map[string][]string
	if s.albums != nil && len(page) > 0 {
		keys := make([]string, len(page))
		for i, it := range page {
			keys[i] = it.MediaKey
		}
		tags, _ = s.albums.TagsFor(ctx, rt.ID, keys)
	}
	items := make([]apiMediaItem, 0, len(page))
	for _, it := range page {
		ai := s.toAPIItem(it)
		ai.Tags = tags[it.MediaKey]
		items = append(items, ai)
	}
	next, prev := "", ""
	if end > start && end < len(src) {
		next = encodeCursor(src[end-1])
	}
	if start > 0 && start < len(src) {
		prev = encodeCursor(src[start])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"nextCursor": next,
		"prevCursor": prev,
		"offset":     start,
		"total":      len(src),
	})
}

// toAPIItem describes an item for the API. For an encrypted/disguised photo or
// document whose thumbnail isn't rendered yet it queues the render now, so it's
// likely done by the time the grid asks, and flags it so clients retry.
func (s *Server) toAPIItem(it gpmc.MediaItem) apiMediaItem {
	display, class, disguised := classifyItem(it.Filename, it.Kind)
	ai := apiMediaItem{
		Key:       it.MediaKey,
		Name:      display,
		Class:     string(class),
		Encrypted: disguised && class != classFile,
		Size:      it.SizeBytes,
		Mtime:     it.Mtime,
		Thumb:     "/api/v1/media/" + it.MediaKey + "/thumb",
		Original:  "/api/v1/media/" + it.MediaKey + "/original",
	}
	if disguised {
		if kind := thumbKindFor(display, class); kind != "" &&
			!s.thumbCached(it.MediaKey, thumbSizes[0], kind) && !s.thumbRecentlyFailed(it.MediaKey) {
			// False when thumbnail generation is off (or the queue is full).
			ai.ThumbPending = s.queueThumb(thumbJob{key: it.MediaKey, kind: kind})
		}
	}
	return ai
}

// apiMediaTimeline returns how many items each month has, newest first, for
// the library's date scrubber. ?tz=<IANA zone> groups by the viewer's own
// months; ?album and ?tag narrow it like the media list.
func (s *Server) apiMediaTimeline(w http.ResponseWriter, r *http.Request) {
	rt := s.accts.Active()
	if rt == nil || rt.Lib == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "library unavailable")
		return
	}
	ctx, cancel := withTimeout(r.Context(), 25*time.Second)
	defer cancel()
	src, lerr := s.listingFor(ctx, rt, r)
	if lerr != nil {
		writeAPIError(w, lerr.code, lerr.msg)
		return
	}
	loc, err := time.LoadLocation(r.URL.Query().Get("tz"))
	if err != nil {
		loc = time.UTC
	}
	type month struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Count int `json:"count"`
	}
	months := []month{}
	for _, it := range src {
		t := it.Mtime.In(loc)
		if n := len(months); n > 0 && months[n-1].Year == t.Year() && months[n-1].Month == int(t.Month()) {
			months[n-1].Count++
			continue
		}
		months = append(months, month{t.Year(), int(t.Month()), 1})
	}
	writeJSON(w, http.StatusOK, map[string]any{"months": months, "total": len(src)})
}
