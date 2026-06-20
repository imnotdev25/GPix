package web

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	// Register decoders for the formats we can thumbnail in pure Go.
	_ "image/gif"
	_ "image/png"

	"gpix/pkg/disguise"
	"gpix/pkg/mediacrypt"
)

const thumbCacheMaxBytes = 40 << 20 // don't try to thumbnail decrypted images larger than this

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
	url, err := s.urlCache.Get(ctx, mediaKey)
	if err != nil {
		return nil, "", err
	}
	resp, _, err := s.doProxyGet(ctx, url, mediaKey, false)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode == 403 || resp.StatusCode == 410 {
		resp.Body.Close()
		s.urlCache.Invalidate(mediaKey)
		if url, err = s.urlCache.Get(ctx, mediaKey); err != nil {
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
func (s *Server) thumbCachePath(mediaKey string, size int) string {
	sum := sha256.Sum256([]byte(mediaKey))
	return filepath.Join(s.thumbDir, fmt.Sprintf("%s_%d.jpg", hex.EncodeToString(sum[:16]), size))
}

// serveGeneratedThumb decrypts the original photo, builds a JPEG thumbnail,
// caches it on disk, and writes it. Returns false if it could not generate one
// (caller should fall back to the upstream thumbnail).
func (s *Server) serveGeneratedThumb(w http.ResponseWriter, r *http.Request, mediaKey, name string, size int) bool {
	if s.thumbDir == "" || !canGenerateThumb(name) {
		return false
	}
	path := s.thumbCachePath(mediaKey, size)
	if data, err := os.ReadFile(path); err == nil {
		writeThumb(w, data)
		return true
	}

	// Single-flight per cache key so a burst of grid requests generates once.
	unlock := s.thumbLock(path)
	defer unlock()
	if data, err := os.ReadFile(path); err == nil { // someone generated it while we waited
		writeThumb(w, data)
		return true
	}

	ctx, cancel := withTimeout(r.Context(), 90*time.Second)
	defer cancel()
	rc, _, err := s.openDecrypted(ctx, mediaKey)
	if err != nil {
		s.log.Debug("thumb: open", "key", mediaKey, "err", err)
		return false
	}
	defer rc.Close()

	img, _, err := image.Decode(io.LimitReader(rc, thumbCacheMaxBytes))
	if err != nil {
		s.log.Debug("thumb: decode", "name", name, "err", err)
		return false
	}
	thumb := downscale(img, size)

	var buf []byte
	bw := &byteWriter{}
	if err := jpeg.Encode(bw, thumb, &jpeg.Options{Quality: 82}); err != nil {
		return false
	}
	buf = bw.b
	_ = os.WriteFile(path, buf, 0o600) // best-effort cache write
	writeThumb(w, buf)
	return true
}

func writeThumb(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	_, _ = w.Write(data)
}

// thumbLock returns an unlock func, serialising generation per cache path.
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
