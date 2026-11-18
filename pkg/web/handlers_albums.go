package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gpix/pkg/accounts"
	"gpix/pkg/albums"
	"gpix/pkg/gpmc"
)

// Albums and tags are gpix's own, stored in albums.db per linked account.
// They aren't synced with albums in Google Photos.

type apiAlbum struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"`
	Count     int           `json:"count"`
	Cover     *apiMediaItem `json:"cover,omitempty"`
	CreatedAt time.Time     `json:"createdAt"`
}

type collectionBody struct {
	Name string   `json:"name"`
	Keys []string `json:"keys"`
	Tags []string `json:"tags"`
}

const maxCollectionKeys = 5000

func (s *Server) collectionRuntime(w http.ResponseWriter) *accounts.Runtime {
	rt := s.accts.Active()
	if s.albums == nil || rt == nil || rt.Lib == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "albums unavailable")
		return nil
	}
	return rt
}

func readCollectionBody(w http.ResponseWriter, r *http.Request) (collectionBody, bool) {
	var b collectionBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&b); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad json")
		return b, false
	}
	if len(b.Keys) > maxCollectionKeys {
		writeAPIError(w, http.StatusBadRequest, "too many items in one request")
		return b, false
	}
	b.Name = strings.TrimSpace(b.Name)
	if r := []rune(b.Name); len(r) > 100 {
		b.Name = string(r[:100])
	}
	return b, true
}

func albumID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad album id")
		return 0, false
	}
	return id, true
}

func writeAlbumErr(w http.ResponseWriter, err error) {
	if errors.Is(err, albums.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "no such album")
		return
	}
	writeAPIError(w, http.StatusInternalServerError, err.Error())
}

// albumView counts an album's items that are still in the library and uses
// the newest as its cover.
func (s *Server) albumView(rt *accounts.Runtime, a albums.Album, keys []string) apiAlbum {
	v := apiAlbum{ID: a.ID, Name: a.Name, CreatedAt: a.CreatedAt}
	var newest gpmc.MediaItem
	for _, k := range keys {
		it, ok := rt.Lib.Peek(k)
		if !ok {
			continue
		}
		v.Count++
		if v.Count == 1 || it.Mtime.After(newest.Mtime) {
			newest = it
		}
	}
	if v.Count > 0 {
		c := s.toAPIItem(newest)
		v.Cover = &c
	}
	return v
}

// writeAlbum responds with one album's current summary.
func (s *Server) writeAlbum(w http.ResponseWriter, r *http.Request, rt *accounts.Runtime, id int64) {
	a, err := s.albums.Get(r.Context(), rt.ID, id)
	if err != nil {
		writeAlbumErr(w, err)
		return
	}
	keys, err := s.albums.Keys(r.Context(), rt.ID, id)
	if err != nil {
		writeAlbumErr(w, err)
		return
	}
	list := make([]string, 0, len(keys))
	for k := range keys {
		list = append(list, k)
	}
	writeJSON(w, http.StatusOK, s.albumView(rt, a, list))
}

// warm makes sure the listing is loaded, so counts and covers are right.
func warm(r *http.Request, rt *accounts.Runtime) {
	ctx, cancel := withTimeout(r.Context(), 25*time.Second)
	defer cancel()
	_, _ = rt.Lib.All(ctx)
}

func (s *Server) apiAlbumsList(w http.ResponseWriter, r *http.Request) {
	rt := s.collectionRuntime(w)
	if rt == nil {
		return
	}
	warm(r, rt)
	list, err := s.albums.List(r.Context(), rt.ID)
	if err != nil {
		writeAlbumErr(w, err)
		return
	}
	members, err := s.albums.Membership(r.Context(), rt.ID)
	if err != nil {
		writeAlbumErr(w, err)
		return
	}
	out := make([]apiAlbum, 0, len(list))
	for _, a := range list {
		out = append(out, s.albumView(rt, a, members[a.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"albums": out})
}

func (s *Server) apiAlbumCreate(w http.ResponseWriter, r *http.Request) {
	rt := s.collectionRuntime(w)
	if rt == nil {
		return
	}
	b, ok := readCollectionBody(w, r)
	if !ok {
		return
	}
	if b.Name == "" {
		writeAPIError(w, http.StatusBadRequest, "give the album a name")
		return
	}
	a, err := s.albums.Create(r.Context(), rt.ID, b.Name)
	if err == nil && len(b.Keys) > 0 {
		err = s.albums.AddItems(r.Context(), rt.ID, a.ID, b.Keys)
	}
	if err != nil {
		writeAlbumErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.albumView(rt, a, b.Keys))
}

func (s *Server) apiAlbumGet(w http.ResponseWriter, r *http.Request) {
	rt := s.collectionRuntime(w)
	if rt == nil {
		return
	}
	id, ok := albumID(w, r)
	if !ok {
		return
	}
	warm(r, rt)
	s.writeAlbum(w, r, rt, id)
}

func (s *Server) apiAlbumRename(w http.ResponseWriter, r *http.Request) {
	rt := s.collectionRuntime(w)
	if rt == nil {
		return
	}
	id, ok := albumID(w, r)
	if !ok {
		return
	}
	b, ok := readCollectionBody(w, r)
	if !ok {
		return
	}
	if b.Name == "" {
		writeAPIError(w, http.StatusBadRequest, "give the album a name")
		return
	}
	if err := s.albums.Rename(r.Context(), rt.ID, id, b.Name); err != nil {
		writeAlbumErr(w, err)
		return
	}
	s.writeAlbum(w, r, rt, id)
}

// apiAlbumDelete removes an album; the photos in it stay in the library.
func (s *Server) apiAlbumDelete(w http.ResponseWriter, r *http.Request) {
	rt := s.collectionRuntime(w)
	if rt == nil {
		return
	}
	id, ok := albumID(w, r)
	if !ok {
		return
	}
	if err := s.albums.Delete(r.Context(), rt.ID, id); err != nil {
		writeAlbumErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) apiAlbumAddItems(w http.ResponseWriter, r *http.Request) {
	s.editAlbumItems(w, r, true)
}

func (s *Server) apiAlbumRemoveItems(w http.ResponseWriter, r *http.Request) {
	s.editAlbumItems(w, r, false)
}

func (s *Server) editAlbumItems(w http.ResponseWriter, r *http.Request, add bool) {
	rt := s.collectionRuntime(w)
	if rt == nil {
		return
	}
	id, ok := albumID(w, r)
	if !ok {
		return
	}
	b, ok := readCollectionBody(w, r)
	if !ok {
		return
	}
	if len(b.Keys) == 0 {
		writeAPIError(w, http.StatusBadRequest, "no items")
		return
	}
	var err error
	if add {
		err = s.albums.AddItems(r.Context(), rt.ID, id, b.Keys)
	} else {
		err = s.albums.RemoveItems(r.Context(), rt.ID, id, b.Keys)
	}
	if err != nil {
		writeAlbumErr(w, err)
		return
	}
	s.writeAlbum(w, r, rt, id)
}

// apiTagsList returns the account's tags with how many library items carry
// each, most used first.
func (s *Server) apiTagsList(w http.ResponseWriter, r *http.Request) {
	rt := s.collectionRuntime(w)
	if rt == nil {
		return
	}
	warm(r, rt)
	members, err := s.albums.TagMembership(r.Context(), rt.ID)
	if err != nil {
		writeAlbumErr(w, err)
		return
	}
	type tagCount struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	out := []tagCount{}
	for t, keys := range members {
		n := 0
		for _, k := range keys {
			if _, ok := rt.Lib.Peek(k); ok {
				n++
			}
		}
		if n > 0 {
			out = append(out, tagCount{t, n})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	writeJSON(w, http.StatusOK, map[string]any{"tags": out})
}

func (s *Server) apiTagsAdd(w http.ResponseWriter, r *http.Request)    { s.editTags(w, r, true) }
func (s *Server) apiTagsRemove(w http.ResponseWriter, r *http.Request) { s.editTags(w, r, false) }

// editTags adds or removes tags on items and returns the items' tags after.
func (s *Server) editTags(w http.ResponseWriter, r *http.Request, add bool) {
	rt := s.collectionRuntime(w)
	if rt == nil {
		return
	}
	b, ok := readCollectionBody(w, r)
	if !ok {
		return
	}
	var tags []string
	for _, t := range b.Tags {
		if t = albums.NormalizeTag(t); t != "" && !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	if len(b.Keys) == 0 || len(tags) == 0 {
		writeAPIError(w, http.StatusBadRequest, "choose items and at least one tag")
		return
	}
	if len(tags) > 20 {
		writeAPIError(w, http.StatusBadRequest, "at most 20 tags at a time")
		return
	}
	var err error
	if add {
		err = s.albums.AddTags(r.Context(), rt.ID, b.Keys, tags)
	} else {
		err = s.albums.RemoveTags(r.Context(), rt.ID, b.Keys, tags)
	}
	if err != nil {
		writeAlbumErr(w, err)
		return
	}
	now, err := s.albums.TagsFor(r.Context(), rt.ID, b.Keys)
	if err != nil {
		writeAlbumErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": now})
}
