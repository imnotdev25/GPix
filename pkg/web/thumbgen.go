package web

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	// Register the GIF decoder (JPEG and PNG decoders come from their imports above).
	_ "image/gif"

	"gpix/pkg/disguise"
	"gpix/pkg/mediacrypt"
)

const thumbCacheMaxBytes = 40 << 20 // don't try to thumbnail decrypted images larger than this

// thumbSizes are the sizes gpix renders for an encrypted/disguised item, all
// from one download + decrypt + decode: the grid (512, 256 for the app) and the
// full-screen viewer (2048). Decrypting the original is the slow part, so it
// happens once per item instead of once per size.
var thumbSizes = []int{512, 256, 2048}

// readCloser ties a logical reader to the underlying closer.
type webReadCloser struct {
	r io.Reader
	c io.Closer
}

func (rc webReadCloser) Read(p []byte) (int, error) { return rc.r.Read(p) }
func (rc webReadCloser) Close() error               { return rc.c.Close() }

type webMultiCloser struct {
	r       io.Reader
	closers []io.Closer
}

func (m webMultiCloser) Read(p []byte) (int, error) { return m.r.Read(p) }
func (m webMultiCloser) Close() error {
	var first error
	for _, c := range m.closers {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// openDecrypted opens the true original bytes for a media key: it resolves the
// download URL, un-disguises and decrypts as needed. The caller closes it.
func (s *Server) openDecrypted(ctx context.Context, mediaKey string) (io.ReadCloser, string, error) {
	url, err := s.urls().Get(ctx, mediaKey)
	if err != nil {
		return nil, "", err
	}
	resp, _, err := s.doProxyGet(ctx, url, mediaKey, false)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode == 403 || resp.StatusCode == 410 {
		resp.Body.Close()
		s.urls().Invalidate(mediaKey)
		if url, err = s.urls().Get(ctx, mediaKey); err != nil {
			return nil, "", err
		}
		if resp, _, err = s.doProxyGet(ctx, url, mediaKey, true); err != nil {
			return nil, "", err
		}
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("openDecrypted: upstream %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	br := bufio.NewReaderSize(resp.Body, 64*1024)
	head, _ := br.Peek(8192)
	if disguise.LooksDisguised(head) {
		hdr, payload, derr := disguise.Extract(br)
		if derr == nil {
			ct = disguiseMIME(hdr.Filename)
			if s.crypt != nil {
				pbr := bufio.NewReader(payload)
				if ph, _ := pbr.Peek(len(mediacrypt.Magic)); mediacrypt.HasMagic(ph) {
					eh, dr, eerr := s.crypt.DecryptingReader(pbr)
					if eerr != nil {
						resp.Body.Close()
						return nil, "", eerr
					}
					return webMultiCloser{r: dr, closers: []io.Closer{dr, resp.Body}}, disguiseMIME(eh.Name), nil
				}
				return webReadCloser{r: pbr, c: resp.Body}, ct, nil
			}
			return webReadCloser{r: payload, c: resp.Body}, ct, nil
		}
	}
	return webReadCloser{r: br, c: resp.Body}, ct, nil
}

// thumbCachePath returns the on-disk path for a generated thumbnail.
func (s *Server) thumbCachePath(mediaKey string, size int, ext string) string {
	sum := sha256.Sum256([]byte(mediaKey))
	return filepath.Join(s.thumbDir, fmt.Sprintf("%s_%d.%s", hex.EncodeToString(sum[:16]), size, ext))
}

// thumbKindMeta maps a kind ("photo"|"doc") to its cache extension + MIME type.
func thumbKindMeta(kind string) (ext, contentType string) {
	if kind == "doc" {
		return "png", "image/png"
	}
	return "jpg", "image/jpeg"
}

// thumbKindFor says what gpix renders for a disguised item: "photo", "doc",
// or "" when it can't make a thumbnail.
func thumbKindFor(display string, class mediaClass) string {
	switch {
	case class == classDoc:
		return "doc"
	case class == classPhoto && canGenerateThumb(display):
		return "photo"
	}
	return ""
}

// cachedThumbFile returns a previously generated thumbnail from disk, if present.
func (s *Server) cachedThumbFile(mediaKey string, size int, kind string) ([]byte, string, bool) {
	if s.thumbDir == "" {
		return nil, "", false
	}
	ext, ct := thumbKindMeta(kind)
	if data, err := os.ReadFile(s.thumbCachePath(mediaKey, size, ext)); err == nil {
		return data, ct, true
	}
	return nil, "", false
}

// thumbCached reports whether a thumbnail is on disk, without reading it.
func (s *Server) thumbCached(mediaKey string, size int, kind string) bool {
	if s.thumbDir == "" {
		return false
	}
	ext, _ := thumbKindMeta(kind)
	_, err := os.Stat(s.thumbCachePath(mediaKey, size, ext))
	return err == nil
}

// thumbBytes returns a generated thumbnail, rendering it (with the other
// sizes) first if needed. Only the single-item view calls this directly; the
// grid goes through queueThumb so it never waits on a decrypt.
func (s *Server) thumbBytes(ctx context.Context, mediaKey string, size int, kind string) ([]byte, string, error) {
	if data, ct, ok := s.cachedThumbFile(mediaKey, size, kind); ok {
		return data, ct, nil
	}
	if err := s.renderThumbs(ctx, mediaKey, kind, nil, size); err != nil {
		return nil, "", err
	}
	if data, ct, ok := s.cachedThumbFile(mediaKey, size, kind); ok {
		return data, ct, nil
	}
	return nil, "", errors.New("thumbnail was not written")
}

// renderThumbs writes every missing thumbnail size (thumbSizes plus any extra)
// for an item from a single read of the original. open supplies the plaintext
// (e.g. a file just uploaded); nil downloads and decrypts the stored original.
// Concurrent calls for one item wait for each other and then find the files.
func (s *Server) renderThumbs(ctx context.Context, mediaKey, kind string, open func() (io.ReadCloser, error), extra ...int) error {
	if s.thumbDir == "" {
		return errors.New("no thumbnail cache directory")
	}
	unlock := s.thumbLock(mediaKey)
	defer unlock()

	ext, _ := thumbKindMeta(kind)
	var missing []int
	for _, sz := range append(slices.Clone(thumbSizes), extra...) {
		if sz <= 0 || slices.Contains(missing, sz) {
			continue
		}
		if _, err := os.Stat(s.thumbCachePath(mediaKey, sz, ext)); err != nil {
			missing = append(missing, sz)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	var (
		rc  io.ReadCloser
		err error
	)
	if open != nil {
		rc, err = open()
	} else {
		rc, _, err = s.openDecrypted(ctx, mediaKey)
	}
	if err != nil {
		return err
	}
	defer rc.Close()

	if kind == "doc" {
		content, err := io.ReadAll(io.LimitReader(rc, 512<<10)) // first 512 KB
		if err != nil {
			return err
		}
		for _, sz := range missing {
			width, maxLines := docRenderParams(sz)
			bw := &byteWriter{}
			if err := png.Encode(bw, renderDocImage(content, width, maxLines)); err != nil {
				return err
			}
			if err := s.writeThumbFile(mediaKey, sz, ext, bw.b); err != nil {
				return err
			}
		}
		return nil
	}

	img, release, err := decodeLimited(ctx, rc)
	if err != nil {
		return err
	}
	// One pass over the full-size original down to the largest size, then the
	// smaller ones from that. Smallest first, so grid tiles fill in soonest.
	slices.Sort(missing)
	big := downscale(img, missing[len(missing)-1])
	release() // the full-size image is no longer needed
	for _, sz := range missing {
		bw := &byteWriter{}
		if err := jpeg.Encode(bw, downscale(big, sz), &jpeg.Options{Quality: 82}); err != nil {
			return err
		}
		if err := s.writeThumbFile(mediaKey, sz, ext, bw.b); err != nil {
			return err
		}
	}
	return nil
}

// writeThumbFile writes a thumbnail atomically, so a reader never sees half a file.
func (s *Server) writeThumbFile(mediaKey string, size int, ext string, data []byte) error {
	tmp, err := os.CreateTemp(s.thumbDir, ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.thumbCachePath(mediaKey, size, ext))
}

// thumbJob is one item for the background thumbnail workers.
type thumbJob struct {
	key, kind string
	extra     int                           // a size outside thumbSizes, or 0
	open      func() (io.ReadCloser, error) // nil: download + decrypt
	cleanup   func()                        // run once the job is done or dropped
}

// queueThumb renders an item's thumbnails in the background (bounded worker
// pool), so a grid request never waits on a download + decrypt. Requests for an
// item that's already queued are merged. It returns false if the queue is full.
func (s *Server) queueThumb(j thumbJob) bool {
	drop := func() {
		if j.cleanup != nil {
			j.cleanup()
		}
	}
	if s.thumbJobs == nil {
		drop()
		return false
	}
	qk := j.key + "|" + strconv.Itoa(j.extra)
	if _, dup := s.thumbQueued.LoadOrStore(qk, true); dup {
		drop()
		return true
	}
	select {
	case s.thumbJobs <- j:
		return true
	default:
		s.thumbQueued.Delete(qk)
		drop()
		s.log.Debug("thumbnail queue full; the grid will ask again", "key", j.key)
		return false
	}
}

// logThumbFailure warns about failed renders, at most 5 a minute; the rest go
// to debug, so a folder of files gpix can't decode doesn't flood the log.
func (s *Server) logThumbFailure(key string, err error) {
	s.thumbLogMu.Lock()
	now := time.Now()
	if now.Sub(s.thumbLogAt) > time.Minute {
		s.thumbLogAt, s.thumbLogN = now, 0
	}
	s.thumbLogN++
	n := s.thumbLogN
	s.thumbLogMu.Unlock()
	switch {
	case n <= 5:
		s.log.Warn("thumbnail render failed; will retry in 10 minutes", "key", key, "err", err)
	case n == 6:
		s.log.Warn("more thumbnail renders failing; further failures this minute are logged at debug level")
	default:
		s.log.Debug("thumbnail render failed", "key", key, "err", err)
	}
}

func (s *Server) thumbWorker() {
	for j := range s.thumbJobs {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		var extra []int
		if j.extra > 0 {
			extra = []int{j.extra}
		}
		start := time.Now()
		if err := s.renderThumbs(ctx, j.key, j.kind, j.open, extra...); err != nil {
			s.thumbFailed.Store(j.key, time.Now())
			s.logThumbFailure(j.key, err)
		} else {
			s.thumbFailed.Delete(j.key)
			s.log.Debug("thumbnail rendered", "key", j.key, "kind", j.kind, "ms", time.Since(start).Milliseconds())
		}
		cancel()
		if j.cleanup != nil {
			j.cleanup()
		}
		s.thumbQueued.Delete(j.key + "|" + strconv.Itoa(j.extra))
	}
}

// thumbRecentlyFailed reports a render that failed in the last 10 minutes, so
// neither gpix nor the client keeps retrying a file it can't decode.
func (s *Server) thumbRecentlyFailed(key string) bool {
	v, ok := s.thumbFailed.Load(key)
	if !ok {
		return false
	}
	if time.Since(v.(time.Time)) > 10*time.Minute {
		s.thumbFailed.Delete(key)
		return false
	}
	return true
}

// thumbWorkers sizes the pool: decoding is CPU-heavy and holds a full-size
// image in memory, so stay between 2 and 6.
func thumbWorkers() int { return min(max(runtime.NumCPU(), 2), 6) }

// docRenderParams maps a requested thumbnail size to a render width and line cap:
// small sizes are grid previews, large sizes are the full-page document view.
func docRenderParams(size int) (width, maxLines int) {
	if size >= 1024 {
		return 1000, 600
	}
	return 560, 40
}

func writeThumb(w http.ResponseWriter, data []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	_, _ = w.Write(data)
}

// thumbLock returns an unlock func, serialising generation per media key.
func (s *Server) thumbLock(key string) func() {
	mu, _ := s.thumbLocks.LoadOrStore(key, &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

type byteWriter struct{ b []byte }

func (bw *byteWriter) Write(p []byte) (int, error) {
	bw.b = append(bw.b, p...)
	return len(p), nil
}

// downscale resizes src to fit within maxDim on its longest side using box
// averaging (good quality for downscaling, pure stdlib).
func downscale(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw == 0 || sh == 0 {
		return src
	}
	scale := 1.0
	if sw > sh {
		if sw > maxDim {
			scale = float64(maxDim) / float64(sw)
		}
	} else {
		if sh > maxDim {
			scale = float64(maxDim) / float64(sh)
		}
	}
	dw, dh := int(float64(sw)*scale), int(float64(sh)*scale)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	if dw == sw && dh == sh {
		return src
	}

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xRatio := float64(sw) / float64(dw)
	yRatio := float64(sh) / float64(dh)
	for dy := 0; dy < dh; dy++ {
		sy0 := b.Min.Y + int(float64(dy)*yRatio)
		sy1 := b.Min.Y + int(float64(dy+1)*yRatio)
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := 0; dx < dw; dx++ {
			sx0 := b.Min.X + int(float64(dx)*xRatio)
			sx1 := b.Min.X + int(float64(dx+1)*xRatio)
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var rs, gs, bs, as, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					rr, gg, bb, aa := src.At(sx, sy).RGBA()
					rs += uint64(rr)
					gs += uint64(gg)
					bs += uint64(bb)
					as += uint64(aa)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			off := dst.PixOffset(dx, dy)
			dst.Pix[off+0] = uint8((rs / n) >> 8)
			dst.Pix[off+1] = uint8((gs / n) >> 8)
			dst.Pix[off+2] = uint8((bs / n) >> 8)
			dst.Pix[off+3] = uint8((as / n) >> 8)
		}
	}
	return dst
}
