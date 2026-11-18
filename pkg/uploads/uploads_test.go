package uploads

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "uploads.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(s.Since()) > time.Minute {
		t.Fatalf("Since = %v, want ~now", s.Since())
	}

	for _, k := range []string{"a", "b", "c"} {
		if err := s.Record(ctx, 1, k, k+".jpg"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Record(ctx, 1, "a", "renamed.jpg"); err != nil { // duplicate is fine
		t.Fatal(err)
	}
	if err := s.Record(ctx, 2, "z", "z.jpg"); err != nil {
		t.Fatal(err)
	}

	keys, err := s.Keys(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 || !keys["a"] || !keys["b"] || !keys["c"] || keys["z"] {
		t.Fatalf("account 1 keys = %v", keys)
	}

	if err := s.Forget(ctx, 1, []string{"a", "missing"}); err != nil {
		t.Fatal(err)
	}
	keys, _ = s.Keys(ctx, 1)
	if len(keys) != 2 || keys["a"] {
		t.Fatalf("after forget, keys = %v", keys)
	}

	since := s.Since()
	s.Close()

	// Reopening keeps the records and the original start time.
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if !s2.Since().Equal(since) {
		t.Fatalf("Since changed on reopen: %v -> %v", since, s2.Since())
	}
	keys, _ = s2.Keys(ctx, 2)
	if len(keys) != 1 || !keys["z"] {
		t.Fatalf("account 2 keys = %v", keys)
	}
}
