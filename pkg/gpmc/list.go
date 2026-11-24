package gpmc

import (
	"context"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	pb "gpix/pkg/gpmc/gpmcpb"
)

type MediaKind int

const (
	KindUnknown MediaKind = iota
	KindPhoto
	KindVideo
)

type MediaItem struct {
	MediaKey    string
	Filename    string
	Kind        MediaKind
	SizeBytes   int64
	Mtime       time.Time
	SHA1        []byte
	ThumbURL    string
	DownloadURL string
}

// DedupKey returns the URL-safe base64 of the SHA-1 (without padding),
// matching Python gpmc utils.urlsafe_base64.
func (m MediaItem) DedupKey() string {
	if len(m.SHA1) == 0 {
		return ""
	}
	return DedupKeyFromSHA1(m.SHA1)
}

type PageResult struct {
	Items     []MediaItem
	NextToken string
	// Raw is how many items Google returned, including trashed ones that
	// Items leaves out. Zero means the listing has caught up.
	Raw int
}

// ListPage returns one page of the library (Google sends up to 500 items a
// page). pageToken is the previous page's NextToken; empty starts at the top.
func (c *Client) ListPage(ctx context.Context, pageToken string) (PageResult, error) {
	body, err := buildLibRequest("", pageToken)
	if err != nil {
		return PageResult{}, fmt.Errorf("gpmc list-page: build request: %w", err)
	}

	respBytes, err := c.doProto(ctx, "lib-state", endpointLibState, body, true, c.language)
	if err != nil {
		return PageResult{}, err
	}

	resp := &pb.LibStateResponse{}
	if err := proto.Unmarshal(respBytes, resp); err != nil {
		return PageResult{}, fmt.Errorf("gpmc list-page: decode: %w", err)
	}

	raw := resp.GetBody().GetItems()
	items := decodeMediaItems(raw)
	// Field 1.1 continues the listing; it is empty on the last page. (Field
	// 1.6, the sync token, only asks for changes, so following it stopped
	// the listing after the first page.)
	next, _ := libReplyTokens(respBytes)
	if next == pageToken {
		next = ""
	}
	return PageResult{Items: items, NextToken: next, Raw: len(raw)}, nil
}

// maxListPages bounds a full listing in case Google keeps handing out tokens.
const maxListPages = 20000

// ListProgress reports how far a full listing has got, after each page.
type ListProgress struct {
	Pages  int
	Items  int
	Capped bool // stopped at maxListPages rather than at the end
}

func (c *Client) ListRecent(ctx context.Context, n int) ([]MediaItem, error) {
	return c.list(ctx, n, nil)
}

// ListAll lists the whole library, calling progress (if set) after each page.
func (c *Client) ListAll(ctx context.Context, progress func(ListProgress)) ([]MediaItem, error) {
	return c.list(ctx, 0, progress)
}

func (c *Client) list(ctx context.Context, n int, progress func(ListProgress)) ([]MediaItem, error) {
	all := []MediaItem{}
	seen := map[string]bool{}
	cursor := ""
	for pages := 0; pages < maxListPages; pages++ {
		page, err := c.ListPage(ctx, cursor)
		if err != nil {
			return nil, err
		}
		added := 0
		for _, it := range page.Items {
			if !seen[it.MediaKey] {
				seen[it.MediaKey] = true
				all = append(all, it)
				added++
			}
		}
		// Google sends up to 500 items a page and an empty NextToken on the
		// last one. The other checks are a safety net in case a page ever
		// brings nothing, or only items we already have.
		done := page.NextToken == "" || page.Raw == 0 || (added == 0 && len(page.Items) > 0) ||
			(n > 0 && len(all) >= n*2)
		if progress != nil {
			progress(ListProgress{Pages: pages + 1, Items: len(all), Capped: !done && pages+1 == maxListPages})
		}
		if done {
			break
		}
		cursor = page.NextToken
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Mtime.After(all[j].Mtime)
	})

	if n > 0 && n < len(all) {
		all = all[:n]
	}
	return all, nil
}

func decodeMediaItems(raw []*pb.MediaItem) []MediaItem {
	items := make([]MediaItem, 0, len(raw))
	for _, r := range raw {
		key := r.GetMediaKey()
		if key == "" {
			continue
		}
		meta := r.GetMeta()
		if meta.GetTrash().GetTrashedAt() != 0 {
			continue
		}
		kind := KindUnknown
		k := r.GetKind()
		switch {
		case k.HasPhoto():
			kind = KindPhoto
		case k.HasVideo():
			kind = KindVideo
		}
		items = append(items, MediaItem{
			MediaKey:  key,
			Filename:  meta.GetFilename(),
			Kind:      kind,
			SizeBytes: meta.GetSizeBytes(),
			Mtime:     time.UnixMilli(meta.GetUtcTimestampMs()),
			SHA1:      meta.GetHashes().GetSha1(),
		})
	}
	return items
}
