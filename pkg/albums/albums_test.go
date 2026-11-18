package albums

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestAlbumsAndTags(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "albums.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	a, err := s.Create(ctx, 1, "Holiday")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddItems(ctx, 1, a.ID, []string{"k1", "k2", "k1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddItems(ctx, 2, a.ID, []string{"k3"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other account's album: err = %v", err)
	}
	keys, err := s.Keys(ctx, 1, a.ID)
	if err != nil || len(keys) != 2 {
		t.Fatalf("keys = %v, %v", keys, err)
	}
	if err := s.RemoveItems(ctx, 1, a.ID, []string{"k1"}); err != nil {
		t.Fatal(err)
	}
	if keys, _ := s.Keys(ctx, 1, a.ID); len(keys) != 1 || !keys["k2"] {
		t.Fatalf("after remove = %v", keys)
	}
	if err := s.Rename(ctx, 1, a.ID, "Trip"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, 1, a.ID); got.Name != "Trip" {
		t.Fatalf("renamed = %q", got.Name)
	}

	if NormalizeTag("  #Beach   Day ") != "beach day" {
		t.Fatalf("normalize = %q", NormalizeTag("  #Beach   Day "))
	}
	if err := s.AddTags(ctx, 1, []string{"k1", "k2"}, []string{"beach"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTags(ctx, 1, []string{"k1"}, []string{"sunset"}); err != nil {
		t.Fatal(err)
	}
	tf, err := s.TagsFor(ctx, 1, []string{"k1", "k2", "k9"})
	if err != nil || len(tf["k1"]) != 2 || len(tf["k2"]) != 1 || len(tf["k9"]) != 0 {
		t.Fatalf("tags for = %v, %v", tf, err)
	}
	if err := s.RemoveTags(ctx, 1, []string{"k1"}, []string{"beach"}); err != nil {
		t.Fatal(err)
	}
	if tk, _ := s.TagKeys(ctx, 1, "beach"); len(tk) != 1 || !tk["k2"] {
		t.Fatalf("beach keys = %v", tk)
	}

	if err := s.Delete(ctx, 1, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Keys(ctx, 1, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted album: err = %v", err)
	}
}
