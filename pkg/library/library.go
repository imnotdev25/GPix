// Package library provides a shared, background-refreshed cache of the Google
// Photos library listing. Listing the whole library is the slow part of S3
// ListObjects and WebDAV PROPFIND, so every surface that needs "all items" or
// "look up by media key" goes through one Cache instead of re-paginating Google
// on each request.
//
// The cache can be backed by a Store (see pkg/cachedb) that persists the last
// snapshot to disk. With a Store, names are served instantly on startup from
// the saved snapshot while a fresh listing loads in the background
// (stale-while-revalidate). Uploads and deletes made through gpix are applied
// to the cache right away (Add/Remove) instead of waiting for the next listing.
package library

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gpix/pkg/gpmc"
)

// Store persists library snapshots (implemented by pkg/cachedb).
type Store interface {
	Load() ([]gpmc.MediaItem, time.Time, error)
	Save(items []gpmc.MediaItem) error
}

// How long an upload added with Add is kept while Google's listing doesn't
// show it yet.
const keepAdded = 10 * time.Minute

type addedItem struct {
	item gpmc.MediaItem
	at   time.Time
}

// Cache holds the most recent full library listing with a TTL and optional
// background refresh + persistence. It is safe for concurrent use.
type Cache struct {
	gp    *gpmc.Client
	ttl   time.Duration
	store Store

	mu        sync.RWMutex
	items     []gpmc.MediaItem // newest-first; replaced, never modified in place
	byKey     map[string]gpmc.MediaItem
	fetchedAt time.Time
	added     map[string]addedItem // uploads the listing doesn't show yet
	unmerged  []gpmc.MediaItem     // added since items was last rebuilt
	removed   map[string]time.Time // deleted through gpix, and when

	flightMu sync.Mutex
	flight   chan struct{} // non-nil while a refresh is in progress
	lastErr  error         // result of the last finished refresh

	log    *slog.Logger
	listed atomic.Bool // a listing has succeeded in this process
}

// SetLogger sets where listings and their errors are reported. Call it before
// Start; without it the cache logs nothing.
func (c *Cache) SetLogger(l *slog.Logger) {
	if l != nil {
		c.log = l
	}
}

func (c *Cache) logger() *slog.Logger {
	if c.log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return c.log
}

// New returns a cache with the given freshness TTL (defaults to 60s) and an
// optional persistent Store (may be nil). When a Store has a saved snapshot it
// is loaded immediately so the cache is warm from the first request.
func New(gp *gpmc.Client, ttl time.Duration, store Store) *Cache {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	c := &Cache{
		gp:    gp,
		ttl:   ttl,
		store: store,
		byKey: map[string]gpmc.MediaItem{},
	}
	if store != nil {
		if items, savedAt, err := store.Load(); err == nil && len(items) > 0 {
			c.set(items, savedAt, savedAt)
		}
	}
	return c
}

// set installs a listing that was started at `started`. Uploads it doesn't show
// yet are kept, and items deleted after it started are dropped. It returns the
// installed slice.
func (c *Cache) set(items []gpmc.MediaItem, at, started time.Time) []gpmc.MediaItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	listed := make(map[string]bool, len(items))
	for _, it := range items {
		listed[it.MediaKey] = true
	}
	for k, a := range c.added {
		if listed[k] || time.Since(a.at) > keepAdded {
			delete(c.added, k)
			continue
		}
		items = append(items, a.item)
	}
	if len(c.removed) > 0 {
		kept := make([]gpmc.MediaItem, 0, len(items))
		for _, it := range items {
			if t, gone := c.removed[it.MediaKey]; gone && t.After(started) {
				continue // deleted after this listing began
			}
			kept = append(kept, it)
		}
		items = kept
		for k, t := range c.removed {
			if !t.After(started) {
				delete(c.removed, k)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Mtime.After(items[j].Mtime) })
	byKey := make(map[string]gpmc.MediaItem, len(items))
	for _, it := range items {
		byKey[it.MediaKey] = it
	}
	c.items = items
	c.byKey = byKey
	c.unmerged = nil
	c.fetchedAt = at
	return items
}

func (c *Cache) fresh() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)+len(c.unmerged) > 0 && time.Since(c.fetchedAt) < c.ttl
}

func (c *Cache) hasData() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)+len(c.unmerged) > 0
}

// ensure makes sure data is available. If fresh, returns immediately. If stale
// but present, it serves the stale data and refreshes in the background
// (stale-while-revalidate). Only the very first load (no data at all) blocks.
func (c *Cache) ensure(ctx context.Context) error {
	if c.fresh() {
		return nil
	}
	if c.hasData() {
		c.startRefresh()
		return nil
	}
	return c.refreshBlocking(ctx)
}

// startRefresh starts a full listing unless one is already running, and returns
// a channel closed when it finishes. The listing isn't tied to any request, so
// a client that gives up doesn't cancel it, and callers that arrive meanwhile
// join it instead of starting another.
func (c *Cache) startRefresh() <-chan struct{} {
	c.flightMu.Lock()
	defer c.flightMu.Unlock()
	if c.flight != nil {
		return c.flight
	}
	ch := make(chan struct{})
	c.flight = ch
	go func() {
		err := c.Refresh(context.Background())
		c.flightMu.Lock()
		c.lastErr = err
		c.flight = nil
		close(ch)
		c.flightMu.Unlock()
	}()
	return ch
}

// refreshBlocking waits (at most until ctx ends) for a full listing.
func (c *Cache) refreshBlocking(ctx context.Context) error {
	done := c.startRefresh()
	select {
	case <-done:
		c.flightMu.Lock()
		defer c.flightMu.Unlock()
		return c.lastErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Refresh unconditionally re-reads the entire library and persists it.
func (c *Cache) Refresh(ctx context.Context) error {
	log := c.logger()
	started := time.Now()
	before := c.count()
	first := !c.listed.Load()
	if first {
		log.Info("library: listing from Google", "cached", before)
	} else {
		log.Debug("library: listing from Google")
	}
	lastLog := started
	items, err := c.gp.ListAll(ctx, func(p gpmc.ListProgress) {
		if p.Capped {
			log.Warn("library: stopped listing at the page limit; the listing may be incomplete", "pages", p.Pages, "items", p.Items)
			return
		}
		// A long first listing reports progress, so it's clear it isn't stuck.
		if time.Since(lastLog) >= 15*time.Second {
			lastLog = time.Now()
			log.Info("library: still listing", "pages", p.Pages, "items", p.Items, "s", int(time.Since(started).Seconds()))
		}
	})
	if err != nil {
		log.Warn("library: listing failed", "err", err, "after_ms", time.Since(started).Milliseconds(), "serving_cached", before)
		return err
	}
	items = c.set(items, time.Now(), started)
	if c.store != nil {
		if serr := c.store.Save(items); serr != nil {
			log.Warn("library: couldn't save the listing to disk", "err", serr)
		}
	}
	c.listed.Store(true)
	lvl := slog.LevelDebug
	if first || len(items) != before {
		lvl = slog.LevelInfo
	}
	log.Log(ctx, lvl, "library: listed", "items", len(items), "change", len(items)-before,
		"ms", time.Since(started).Milliseconds())
	return nil
}

func (c *Cache) count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items) + len(c.unmerged)
}

// snapshot returns the current listing, newest first, with recent uploads
// merged in. Callers must not modify it.
func (c *Cache) snapshot() []gpmc.MediaItem {
	c.mu.RLock()
	if len(c.unmerged) == 0 {
		items := c.items
		c.mu.RUnlock()
		return items
	}
	c.mu.RUnlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.unmerged) > 0 {
		merged := make([]gpmc.MediaItem, 0, len(c.items)+len(c.unmerged))
		merged = append(merged, c.items...)
		merged = append(merged, c.unmerged...)
		sort.SliceStable(merged, func(i, j int) bool { return merged[i].Mtime.After(merged[j].Mtime) })
		c.items = merged
		c.unmerged = nil
	}
	return c.items
}

// All returns the cached library (refreshing if stale). The slice must not be
// mutated by the caller.
func (c *Cache) All(ctx context.Context) ([]gpmc.MediaItem, error) {
	if err := c.ensure(ctx); err != nil {
		return nil, err
	}
	return c.snapshot(), nil
}

// Add records an item gpix just uploaded so it is listed immediately, before
// Google's listing includes it.
func (c *Cache) Add(it gpmc.MediaItem) {
	if it.MediaKey == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.byKey[it.MediaKey]; ok {
		return
	}
	if c.added == nil {
		c.added = map[string]addedItem{}
	}
	c.added[it.MediaKey] = addedItem{item: it, at: time.Now()}
	c.unmerged = append(c.unmerged, it)
	c.byKey[it.MediaKey] = it
}

// Remove drops items gpix just deleted, so they disappear without waiting for
// the next listing (and don't reappear from one that was already running).
func (c *Cache) Remove(keys ...string) {
	if len(keys) == 0 {
		return
	}
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	keep := func(list []gpmc.MediaItem) []gpmc.MediaItem {
		out := make([]gpmc.MediaItem, 0, len(list))
		for _, it := range list {
			if !drop[it.MediaKey] {
				out = append(out, it)
			}
		}
		return out
	}
	c.items = keep(c.items)
	if len(c.unmerged) > 0 {
		c.unmerged = keep(c.unmerged)
	}
	if c.removed == nil {
		c.removed = map[string]time.Time{}
	}
	now := time.Now()
	for k := range drop {
		delete(c.byKey, k)
		delete(c.added, k)
		c.removed[k] = now
	}
}

// Get looks up a single item by media key (refreshing if stale).
func (c *Cache) Get(ctx context.Context, mediaKey string) (gpmc.MediaItem, bool, error) {
	c.mu.RLock()
	it, ok := c.byKey[mediaKey]
	c.mu.RUnlock()
	if ok && c.fresh() {
		return it, true, nil
	}
	if err := c.ensure(ctx); err != nil {
		return gpmc.MediaItem{}, false, err
	}
	c.mu.RLock()
	it, ok = c.byKey[mediaKey]
	c.mu.RUnlock()
	return it, ok, nil
}

// Peek returns an item from whatever is currently in memory WITHOUT triggering
// a refresh. Use it on hot paths (e.g. per-thumbnail requests) that must never
// block on a full library listing; a miss just means "not known yet".
func (c *Cache) Peek(mediaKey string) (gpmc.MediaItem, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	it, ok := c.byKey[mediaKey]
	return it, ok
}

// WarmUp blocks until the cache has data or maxWait elapses. Startup calls it
// (right after Start, which kicks an eager refresh) so the first S3/WebDAV
// listing serves from memory instead of triggering a cold, full walk of the
// Google library while a client waits and times out. It only waits for the
// refresh Start already launched — it never starts its own — so a very large
// first-run library just keeps warming in the background past the deadline.
// Returns whether the cache ended up warm. A persisted snapshot (see cachedb)
// makes this return immediately on every run after the first.
func (c *Cache) WarmUp(ctx context.Context, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for !c.hasData() {
		if ctx.Err() != nil || time.Now().After(deadline) {
			return c.hasData()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return true
}

// Invalidate forces the next access to refresh (call after uploads/deletes).
func (c *Cache) Invalidate() {
	c.mu.Lock()
	c.fetchedAt = time.Time{}
	c.mu.Unlock()
}

// Start runs a background refresh loop until ctx is cancelled, keeping the cache
// warm. interval defaults to the TTL. A tick is skipped while the previous
// listing is still running.
func (c *Cache) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = c.ttl
	}
	go func() {
		done := c.startRefresh()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				select {
				case <-done:
					done = c.startRefresh()
				default:
				}
			}
		}
	}()
}
