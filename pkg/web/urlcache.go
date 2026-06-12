package web

import (
	"context"
	"net/url"
	"strconv"
	"sync"
	"time"

	"gpix/pkg/gpmc"
)

type urlEntry struct {
	url string
	exp time.Time
}

type urlCache struct {
	gp     *gpmc.Client
	mu     sync.Mutex
	m      map[string]urlEntry
	inFlight map[string]chan struct{}
}

func newURLCache(gp *gpmc.Client) *urlCache {
	return &urlCache{
		gp:       gp,
		m:        make(map[string]urlEntry),
		inFlight: make(map[string]chan struct{}),
	}
}

func (c *urlCache) Get(ctx context.Context, mediaKey string) (string, error) {
	for {
		c.mu.Lock()
		if e, ok := c.m[mediaKey]; ok && time.Now().Before(e.exp) {
			c.mu.Unlock()
			return e.url, nil
		}
		if ch, busy := c.inFlight[mediaKey]; busy {
			c.mu.Unlock()
			select {
			case <-ch:
				continue
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		ch := make(chan struct{})
		c.inFlight[mediaKey] = ch
		c.mu.Unlock()

		orig, _, err := c.gp.GetDownloadURL(ctx, mediaKey)

		c.mu.Lock()
		delete(c.inFlight, mediaKey)
		close(ch)
		if err == nil && orig != "" {
			c.m[mediaKey] = urlEntry{url: orig, exp: parseExpiry(orig)}
		}
		c.mu.Unlock()

		if err != nil {
			return "", err
		}
		return orig, nil
	}
}

func (c *urlCache) Invalidate(mediaKey string) {
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
