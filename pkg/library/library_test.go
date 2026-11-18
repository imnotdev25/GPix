package library

import (
	"testing"
	"time"

	"gpix/pkg/gpmc"
)

func item(key string, minutesAgo int) gpmc.MediaItem {
	return gpmc.MediaItem{MediaKey: key, Mtime: time.Now().Add(-time.Duration(minutesAgo) * time.Minute)}
}

func keys(items []gpmc.MediaItem) string {
	s := ""
	for _, it := range items {
		s += it.MediaKey
	}
	return s
}

func TestAddRemoveAndRefreshMerge(t *testing.T) {
	c := New(nil, 0, nil)
	start := time.Now()
	c.set([]gpmc.MediaItem{item("c", 30), item("a", 10)}, start, start)
	if got := keys(c.snapshot()); got != "ac" {
		t.Fatalf("initial order = %q", got)
	}

	c.Add(item("b", 20)) // an upload with an older capture date lands in order
	c.Add(item("a", 10)) // already listed: ignored
	if got := keys(c.snapshot()); got != "abc" {
		t.Fatalf("after add = %q", got)
	}
	if _, ok := c.Peek("b"); !ok {
		t.Fatal("added item not found by Peek")
	}

	// A listing that started before the upload doesn't show it yet: keep it.
	c.set([]gpmc.MediaItem{item("c", 30), item("a", 10)}, time.Now(), start)
	if got := keys(c.snapshot()); got != "abc" {
		t.Fatalf("after stale refresh = %q", got)
	}

	c.Remove("a")
	if got := keys(c.snapshot()); got != "bc" {
		t.Fatalf("after remove = %q", got)
	}
	// A listing started before the delete still shows it: drop it anyway.
	c.set([]gpmc.MediaItem{item("c", 30), item("b", 20), item("a", 10)}, time.Now(), start)
	if got := keys(c.snapshot()); got != "bc" {
		t.Fatalf("after refresh that predates the delete = %q", got)
	}
	// A listing started after the delete is trusted.
	later := time.Now().Add(time.Second)
	c.set([]gpmc.MediaItem{item("c", 30), item("b", 20)}, later, later)
	if got := keys(c.snapshot()); got != "bc" {
		t.Fatalf("after fresh refresh = %q", got)
	}
}
