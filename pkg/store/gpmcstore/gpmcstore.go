// Package gpmcstore implements store.Backend on top of a Google Photos client
// (pkg/gpmc). It is the production backend for the S3 and WebDAV gateways.
//
// Mapping decisions
//
//   - Flat namespace. Google Photos has no folders, so the store exposes one
//     implicit bucket of objects keyed by filename.
//   - Object key == the item's display filename. Files that gpix disguised as
//     MP4 (see pkg/disguise) are presented under their original name, e.g. a
//     "report.pdf" stored as "report.pdf.mp4" lists as key "report.pdf" and is
//     transparently unwrapped on GET.
//   - Duplicate filenames. Google Photos can hold many items with the same
//     filename. The store keeps the most recent one per key (the library is
//     listed newest-first); older duplicates are not individually addressable.
//   - ETag == hex SHA-1 of the logical object. This is NOT an MD5, so it will
//     not match clients that try to verify ETag==Content-MD5 (most don't).
package gpmcstore

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"gpix/pkg/disguise"
	"gpix/pkg/gpmc"
	"gpix/pkg/store"
)

// Options configures a Backend.
type Options struct {
	// TempDir is where Put buffers incoming bytes before upload. Empty uses the
	// OS default.
	TempDir string
	// Quality is the upload quality passed to gpmc. Defaults to original.
	Quality gpmc.Quality
	// ListTTL is how long a library listing is cached for key→media-key
	// resolution. Defaults to 15s.
	ListTTL time.Duration
}

// Backend is a store.Backend backed by Google Photos.
type Backend struct {
	gp   *gpmc.Client
	opts Options

	mu        sync.Mutex
	index     map[string]gpmc.MediaItem // display key -> newest media item
	indexedAt time.Time

	urls urlCache
}

// New returns a Google-Photos-backed store.
func New(gp *gpmc.Client, opts Options) *Backend {
	if opts.ListTTL <= 0 {
		opts.ListTTL = 15 * time.Second
	}
	return &Backend{
		gp:    gp,
		opts:  opts,
		index: map[string]gpmc.MediaItem{},
		urls:  urlCache{gp: gp, m: map[string]urlEntry{}},
	}
}

func (b *Backend) Name() string { return "google-photos" }

// displayKey returns the object key a media item is exposed under: its original
// filename, with the disguise ".mp4" suffix stripped when present.
func displayKey(it gpmc.MediaItem) string {
	if orig, ok := disguise.LooksLikeDisguisedFilename(it.Filename); ok {
		return orig
	}
	return it.Filename
}

func contentTypeFor(key string, kind gpmc.MediaKind) string {
	if ct := mime.TypeByExtension(filepath.Ext(key)); ct != "" {
		return ct
	}
	switch kind {
	case gpmc.KindPhoto:
		return "image/jpeg"
	case gpmc.KindVideo:
		return "video/mp4"
	}
	return "application/octet-stream"
}

func objectFromItem(it gpmc.MediaItem) store.Object {
	key := displayKey(it)
	return store.Object{
		Key:         key,
		Size:        it.SizeBytes,
		ModTime:     it.Mtime,
		ETag:        hex.EncodeToString(it.SHA1),
		ContentType: contentTypeFor(key, it.Kind),
	}
}

// refreshIndex rebuilds the key→item map from a full library listing if the
// cached copy is older than ListTTL (or force is set).
func (b *Backend) refreshIndex(ctx context.Context, force bool) error {
	b.mu.Lock()
	fresh := !force && time.Since(b.indexedAt) < b.opts.ListTTL && len(b.index) > 0
	b.mu.Unlock()
	if fresh {
		return nil
	}

	items, err := b.gp.ListRecent(ctx, 0)
	if err != nil {
		return fmt.Errorf("gpmcstore: list library: %w", err)
	}
	// items are newest-first; first occurrence of a key wins.
	idx := make(map[string]gpmc.MediaItem, len(items))
	for _, it := range items {
		k := displayKey(it)
		if _, seen := idx[k]; !seen {
			idx[k] = it
		}
	}
	b.mu.Lock()
	b.index = idx
	b.indexedAt = time.Now()
	b.mu.Unlock()
	return nil
}

func (b *Backend) lookup(ctx context.Context, key string) (gpmc.MediaItem, error) {
	b.mu.Lock()
	it, ok := b.index[key]
	b.mu.Unlock()
	if ok {
		return it, nil
	}
	if err := b.refreshIndex(ctx, true); err != nil {
		return gpmc.MediaItem{}, err
	}
	b.mu.Lock()
	it, ok = b.index[key]
	b.mu.Unlock()
	if !ok {
		return gpmc.MediaItem{}, store.ErrNotFound
	}
	return it, nil
}

func (b *Backend) invalidate() {
	b.mu.Lock()
	b.indexedAt = time.Time{}
	b.mu.Unlock()
}

func (b *Backend) List(ctx context.Context) ([]store.Object, error) {
	if err := b.refreshIndex(ctx, false); err != nil {
		return nil, err
	}
	b.mu.Lock()
	out := make([]store.Object, 0, len(b.index))
	for _, it := range b.index {
		out = append(out, objectFromItem(it))
	}
	b.mu.Unlock()
	return out, nil
}

func (b *Backend) Stat(ctx context.Context, key string) (store.Object, error) {
	it, err := b.lookup(ctx, key)
	if err != nil {
		return store.Object{}, err
	}
	return objectFromItem(it), nil
}

// readCloser ties a logical read stream to the underlying closer (the HTTP
// response body) so callers close the right thing.
type readCloser struct {
	r io.Reader
	c io.Closer
}

func (rc readCloser) Read(p []byte) (int, error) { return rc.r.Read(p) }
func (rc readCloser) Close() error               { return rc.c.Close() }

func (b *Backend) Get(ctx context.Context, key string) (io.ReadCloser, store.Object, error) {
	it, err := b.lookup(ctx, key)
	if err != nil {
		return nil, store.Object{}, err
	}
	obj := objectFromItem(it)

	dlURL, err := b.urls.get(ctx, it.MediaKey)
	if err != nil {
		return nil, store.Object{}, fmt.Errorf("gpmcstore: resolve download url: %w", err)
	}
	resp, err := b.fetch(ctx, dlURL)
	if err != nil {
		// One retry on a stale/expired signed URL.
		b.urls.invalidate(it.MediaKey)
		dlURL, err = b.urls.get(ctx, it.MediaKey)
		if err != nil {
			return nil, store.Object{}, err
		}
		resp, err = b.fetch(ctx, dlURL)
		if err != nil {
			return nil, store.Object{}, err
		}
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, store.Object{}, fmt.Errorf("gpmcstore: download upstream status %d", resp.StatusCode)
	}

	br := bufio.NewReaderSize(resp.Body, 64*1024)
	head, _ := br.Peek(8192)
	if disguise.LooksDisguised(head) {
		hdr, payload, derr := disguise.Extract(br)
		if derr == nil {
			obj.Size = hdr.PayloadSize
			if hdr.Filename != "" {
				obj.ContentType = contentTypeFor(hdr.Filename, gpmc.KindUnknown)
			}
			return readCloser{r: payload, c: resp.Body}, obj, nil
		}
		// fall through: serve raw bytes if extraction failed
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if n, perr := strconv.ParseInt(cl, 10, 64); perr == nil {
			obj.Size = n
		}
	}
	return readCloser{r: br, c: resp.Body}, obj, nil
}

func (b *Backend) fetch(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return b.gp.HTTPClient().Do(req)
}

func (b *Backend) Put(ctx context.Context, key string, r io.Reader, _ int64, contentType string) (store.Object, error) {
	// Buffer to a temp file: gpmc.UploadFile needs a real path and stats it.
	tf, err := os.CreateTemp(b.opts.TempDir, "gpix-put-*"+filepath.Ext(key))
	if err != nil {
		return store.Object{}, err
	}
	tmpPath := tf.Name()
	defer os.Remove(tmpPath)

	h := sha1.New()
	size, err := io.Copy(io.MultiWriter(tf, h), r)
	if err != nil {
		tf.Close()
		return store.Object{}, err
	}
	tf.Close()
	etag := hex.EncodeToString(h.Sum(nil))

	uploadPath := tmpPath
	commitName := key
	if head, herr := readHead(tmpPath, 512); herr == nil && disguise.ShouldWrap(contentType, key, head) {
		wrapped, werr := wrapToTemp(b.opts.TempDir, tmpPath, key)
		if werr != nil {
			return store.Object{}, fmt.Errorf("gpmcstore: disguise wrap: %w", werr)
		}
		defer os.Remove(wrapped)
		uploadPath = wrapped
		commitName = key + ".mp4"
	}

	res, err := b.gp.UploadFile(ctx, uploadPath, gpmc.UploadOpts{
		Quality:      b.opts.Quality,
		OverrideName: commitName,
	})
	if err != nil {
		return store.Object{}, fmt.Errorf("gpmcstore: upload: %w", err)
	}
	if res.Err != nil {
		return store.Object{}, fmt.Errorf("gpmcstore: upload: %w", res.Err)
	}
	b.invalidate()

	if contentType == "" {
		contentType = contentTypeFor(key, gpmc.KindUnknown)
	}
	return store.Object{
		Key:         key,
		Size:        size,
		ModTime:     time.Now().UTC(),
		ETag:        etag,
		ContentType: contentType,
	}, nil
}

func (b *Backend) Delete(ctx context.Context, key string) error {
	it, err := b.lookup(ctx, key)
	if err != nil {
		return err
	}
	results, err := b.gp.DeleteByMediaKeys(ctx, []string{it.MediaKey}, false)
	if err != nil {
		return fmt.Errorf("gpmcstore: delete: %w", err)
	}
	if e := results[it.MediaKey]; e != nil {
		return fmt.Errorf("gpmcstore: delete %q: %w", key, e)
	}
	b.invalidate()
	return nil
}

func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	read, err := io.ReadFull(f, buf)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		return buf[:read], nil
	}
	if err != nil {
		return nil, err
	}
	return buf[:read], nil
}

func wrapToTemp(tempDir, srcPath, name string) (string, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return "", err
	}
	out, err := os.CreateTemp(tempDir, "gpix-disg-*.mp4")
	if err != nil {
		return "", err
	}
	defer out.Close()
	wrapped, _ := disguise.Wrap(name, src, st.Size())
	if _, err := io.Copy(out, wrapped); err != nil {
		os.Remove(out.Name())
		return "", err
	}
	return out.Name(), nil
}

// --- signed-download-URL cache (mirrors pkg/web/urlcache.go) ---

type urlEntry struct {
	url string
	exp time.Time
}

type urlCache struct {
	gp *gpmc.Client
	mu sync.Mutex
	m  map[string]urlEntry
}

func (c *urlCache) get(ctx context.Context, mediaKey string) (string, error) {
	c.mu.Lock()
	if e, ok := c.m[mediaKey]; ok && time.Now().Before(e.exp) {
		c.mu.Unlock()
		return e.url, nil
	}
	c.mu.Unlock()

	orig, _, err := c.gp.GetDownloadURL(ctx, mediaKey)
	if err != nil {
		return "", err
	}
	if orig == "" {
		return "", fmt.Errorf("gpmcstore: empty download url for %q", mediaKey)
	}
	c.mu.Lock()
	c.m[mediaKey] = urlEntry{url: orig, exp: parseExpiry(orig)}
	c.mu.Unlock()
	return orig, nil
}

func (c *urlCache) invalidate(mediaKey string) {
	c.mu.Lock()
	delete(c.m, mediaKey)
	c.mu.Unlock()
}

func parseExpiry(rawURL string) time.Time {
	u, err := url.Parse(rawURL)
	if err != nil {
		return time.Now().Add(5 * time.Minute)
	}
	if v := u.Query().Get("expire"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Unix(n, 0).Add(-30 * time.Second)
		}
	}
	return time.Now().Add(5 * time.Minute)
}

// Ensure Backend satisfies the interface at compile time.
var _ store.Backend = (*Backend)(nil)
