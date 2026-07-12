package immichimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"gpix/pkg/gpmc"
)

// srcItem is one item to import (an opaque id plus a display name).
type srcItem struct {
	id   string
	name string
}

// source is an import origin — an Immich server or another Google account. It
// enumerates items and downloads their original bytes; the import engine handles
// dedup, encryption/disguise, and upload the same way for every source.
type source interface {
	// label is a stable key for the imported-IDs table (so re-runs are cheap).
	label() string
	ping(ctx context.Context) error
	// enumerate reports the total via setTotal as soon as it's known, then calls
	// emit for each item (emit returns false to stop early, e.g. on cancel).
	enumerate(ctx context.Context, setTotal func(int), emit func(srcItem) bool) error
	download(ctx context.Context, id string) (io.ReadCloser, string, error)
}

// ---- Immich source ----

type immichSource struct{ c *Client }

func (s immichSource) label() string                  { return "immich:" + s.c.baseURL }
func (s immichSource) ping(ctx context.Context) error { return s.c.Ping(ctx) }

func (s immichSource) enumerate(ctx context.Context, setTotal func(int), emit func(srcItem) bool) error {
	buckets, err := s.c.Buckets(ctx)
	if err != nil {
		return err
	}
	total := 0
	for _, b := range buckets {
		total += b.Count
	}
	setTotal(total)
	for _, b := range buckets {
		if ctx.Err() != nil {
			return nil
		}
		assets, aerr := s.c.BucketAssets(ctx, b.TimeBucket)
		if aerr != nil {
			continue // skip a bad bucket, keep going
		}
		for _, a := range assets {
			if !emit(srcItem{id: a.ID, name: a.OriginalFileName}) {
				return nil
			}
		}
	}
	return nil
}

func (s immichSource) download(ctx context.Context, id string) (io.ReadCloser, string, error) {
	return s.c.Download(ctx, id)
}

// ---- Google account source ----

// googleSource imports from another Google Photos account given its GP_AUTH_DATA.
type googleSource struct {
	gp  *gpmc.Client
	lbl string
}

func newGoogleSource(authData string) (*googleSource, error) {
	gp, err := gpmc.New(authData)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(authData))
	return &googleSource{gp: gp, lbl: "google:" + hex.EncodeToString(sum[:8])}, nil
}

func (g *googleSource) label() string { return g.lbl }

func (g *googleSource) ping(ctx context.Context) error {
	_, err := g.gp.BearerToken(ctx) // validates auth_data
	return err
}

func (g *googleSource) enumerate(ctx context.Context, setTotal func(int), emit func(srcItem) bool) error {
	items, err := g.gp.ListRecent(ctx, 0) // 0 = whole library
	if err != nil {
		return err
	}
	setTotal(len(items))
	for _, it := range items {
		if !emit(srcItem{id: it.MediaKey, name: it.Filename}) {
			return nil
		}
	}
	return nil
}

func (g *googleSource) download(ctx context.Context, mediaKey string) (io.ReadCloser, string, error) {
	orig, _, err := g.gp.GetDownloadURL(ctx, mediaKey)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, orig, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := g.gp.HTTPClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("download %s: status %d", mediaKey, resp.StatusCode)
	}
	return resp.Body, "", nil // filename comes from the item's name
}
