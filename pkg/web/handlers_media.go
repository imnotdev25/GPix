package web

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"gpix/pkg/disguise"
	"gpix/pkg/gpmc"
	"gpix/pkg/mediacrypt"
)

func (s *Server) handleThumb(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.NotFound(w, r)
		return
	}
	s.serveThumb(w, r, key, thumbSize(r))
}

func thumbSize(r *http.Request) int {
	size := 256
	if v := r.URL.Query().Get("size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			switch n {
			case 64, 128, 256, 512, 1024, 2048:
				size = n
			}
		}
	}
	return size
}

// pendingThumbGIF is a 1×1 transparent GIF served while an encrypted item's
// real thumbnail renders in the background. Clients retry until a real image
// arrives; Cache-Control: no-store keeps anything from caching it.
var pendingThumbGIF = func() []byte {
	var b bytes.Buffer
	_ = gif.Encode(&b, image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Transparent}), nil)
	return b.Bytes()
}()

func (s *Server) serveThumb(w http.ResponseWriter, r *http.Request, key string, size int) {
	// Encrypted/disguised items have only a blank thumbnail on Google's side, so
	// gpix renders them locally (photos from the decrypted original, documents
	// as a text image), every size from one decrypt, and caches them. A cache
	// hit is served instantly. The grid never waits: it gets a placeholder while
	// the item renders in the background. The single-item view renders now.
	generated := false
	if _, kind, ok := s.thumbGenTarget(r, key); ok {
		generated = true
		if data, ct, cached := s.cachedThumbFile(key, size, kind); cached {
			writeThumb(w, data, ct)
			return
		}
		if s.cfg.ThumbGen && !s.thumbRecentlyFailed(key) {
			if size >= 1024 {
				ctx, cancel := withTimeout(r.Context(), 90*time.Second)
				data, ct, err := s.thumbBytes(ctx, key, size, kind)
				cancel()
				if err == nil {
					writeThumb(w, data, ct)
					return
				}
				if r.Context().Err() == nil {
					s.thumbFailed.Store(key, time.Now())
					s.logThumbFailure(key, err)
				}
			} else {
				extra := 0
				if !slices.Contains(thumbSizes, size) {
					extra = size
				}
				s.queueThumb(thumbJob{key: key, kind: kind, extra: extra})
				w.Header().Set("Content-Type", "image/gif")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Thumb-Pending", "1")
				_, _ = w.Write(pendingThumbGIF)
				return
			}
		}
	}

	ctx, cancel := withTimeout(r.Context(), 20*time.Second)
	defer cancel()

	tok, err := s.gpc().BearerToken(ctx)
	if err != nil {
		http.Error(w, "bearer: "+err.Error(), 500)
		return
	}
	url := gpmc.ThumbnailURL(key, size)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := s.gpc().HTTPClient().Do(req)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("thumb upstream %d", resp.StatusCode), resp.StatusCode)
		return
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if generated {
		// gpix couldn't render this one (it failed recently); don't let Google's
		// blank placeholder stick in caches.
		w.Header().Set("Cache-Control", "private, max-age=60")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	}
	_, _ = io.Copy(w, resp.Body)
}

// thumbGenTarget decides whether gpix should render this thumbnail locally
// (an encrypted/disguised photo or document) and returns the display name and
// kind ("photo"/"doc"). It prefers a client hint — ?name=<display>&enc=1 (or
// &local=1) — so thumbnailing works even when the library cache isn't warm;
// otherwise it consults the cache via Peek.
func (s *Server) thumbGenTarget(r *http.Request, key string) (display, kind string, ok bool) {
	q := r.URL.Query()
	if name := q.Get("name"); name != "" && (q.Get("enc") == "1" || q.Get("local") == "1") {
		switch classForExt(name) {
		case classDoc:
			return name, "doc", true
		case classPhoto:
			if canGenerateThumb(name) {
				return name, "photo", true
			}
		}
		return "", "", false
	}
	if lib := s.libc(); lib != nil {
		if it, okc := lib.Peek(key); okc {
			d, class, disguised := classifyItem(it.Filename, it.Kind)
			if disguised {
				switch class {
				case classDoc:
					return d, "doc", true
				case classPhoto:
					if canGenerateThumb(d) {
						return d, "photo", true
					}
				}
			}
		}
	}
	return "", "", false
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	key, err := s.verifyMedia(r.PathValue("token"))
	if err != nil {
		http.Error(w, "bad token", http.StatusForbidden)
		return
	}
	ctx, cancel := withTimeout(r.Context(), 30*time.Second)
	defer cancel()

	manifest, err := s.gpc().GetStreamManifest(ctx, key, "hls")
	if err != nil {
		s.proxyDownload(w, r, key, false)
		return
	}

	levelQ := r.URL.Query().Get("level")
	if levelQ != "" {
		idx, perr := strconv.Atoi(levelQ)
		if perr == nil {
			variants := ParseMasterPlaylist(manifest)
			for _, v := range variants {
				if v.Index == idx {
					http.Redirect(w, r, v.PlaylistURL, http.StatusFound)
					return
				}
			}
		}
		http.Error(w, "unknown level", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = io.WriteString(w, manifest)
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	key, err := s.verifyMedia(r.PathValue("token"))
	if err != nil {
		http.Error(w, "bad token", http.StatusForbidden)
		return
	}
	// inline=1 renders in-browser (img/video) rather than downloading; either
	// way the bytes are un-disguised and decrypted as needed.
	attachment := r.URL.Query().Get("inline") != "1"
	s.proxyDownload(w, r, key, attachment)
}

func (s *Server) proxyDownload(w http.ResponseWriter, r *http.Request, mediaKey string, attachment bool) {
	ctx, cancel := withTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	url, err := s.urls().Get(ctx, mediaKey)
	if err != nil {
		http.Error(w, "resolve: "+err.Error(), http.StatusBadGateway)
		return
	}
	resp, retried, err := s.doProxyGet(ctx, url, mediaKey, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if (resp.StatusCode == 403 || resp.StatusCode == 410) && !retried {
		resp.Body.Close()
		s.urls().Invalidate(mediaKey)
		url, err = s.urls().Get(ctx, mediaKey)
		if err != nil {
			http.Error(w, "re-resolve: "+err.Error(), http.StatusBadGateway)
			return
		}
		resp, _, err = s.doProxyGet(ctx, url, mediaKey, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
	}

	br := bufio.NewReaderSize(resp.Body, 64*1024)
	// Always un-disguise / decrypt a disguised item, whether it's being
	// downloaded (attachment) or rendered inline (img/video). Only the
	// Content-Disposition differs.
	if resp.StatusCode == http.StatusOK {
		head, _ := br.Peek(8192)
		if disguise.LooksDisguised(head) {
			hdr, payload, err := disguise.Extract(br)
			if err == nil {
				// The unwrapped payload may itself be encrypted.
				if s.crypt != nil {
					pbr := bufio.NewReader(payload)
					if ph, _ := pbr.Peek(len(mediacrypt.Magic)); mediacrypt.HasMagic(ph) {
						eh, dr, derr := s.crypt.DecryptingReader(pbr)
						if derr != nil {
							http.Error(w, "decrypt: "+derr.Error(), http.StatusBadGateway)
							return
						}
						writeMediaHeaders(w, eh.Name, eh.OrigSize, attachment)
						_, _ = io.Copy(w, dr)
						dr.Close()
						return
					}
					writeMediaHeaders(w, hdr.Filename, hdr.PayloadSize, attachment)
					_, _ = io.Copy(w, pbr)
					return
				}
				writeMediaHeaders(w, hdr.Filename, hdr.PayloadSize, attachment)
				_, _ = io.Copy(w, payload)
				return
			}
		}
	}

	for _, h := range []string{"Content-Type", "Content-Length"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if attachment {
		if cd := resp.Header.Get("Content-Disposition"); cd != "" {
			w.Header().Set("Content-Disposition", cd)
		} else {
			w.Header().Set("Content-Disposition", "attachment")
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, br)
}

func writeMediaHeaders(w http.ResponseWriter, origName string, size int64, attachment bool) {
	if origName == "" {
		origName = "download.bin"
	}
	w.Header().Set("Content-Type", disguiseMIME(origName))
	if size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	dispType := "inline"
	if attachment {
		dispType = "attachment"
	}
	disp := mime.FormatMediaType(dispType, map[string]string{"filename": origName})
	if disp == "" {
		disp = dispType + `; filename="` + origName + `"`
	}
	w.Header().Set("Content-Disposition", disp)
	w.WriteHeader(http.StatusOK)
}

func disguiseMIME(filename string) string {
	if filename == "" {
		return "application/octet-stream"
	}
	if m := mime.TypeByExtension(filepath.Ext(filename)); m != "" {
		return m
	}
	return "application/octet-stream"
}

func (s *Server) doProxyGet(ctx context.Context, url, mediaKey string, _ bool) (*http.Response, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := s.gpc().HTTPClient().Do(req)
	return resp, false, err
}

func withTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}
