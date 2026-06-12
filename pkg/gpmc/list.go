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
	MediaKey   string
	Filename   string
	Kind       MediaKind
	SizeBytes  int64
	Mtime      time.Time
	SHA1       []byte
	ThumbURL   string
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
}

func (c *Client) ListPage(ctx context.Context, resumeToken string) (PageResult, error) {
	body, err := buildLibStateRequest(resumeToken)
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

	items := decodeMediaItems(resp.GetBody().GetItems())
	next := resp.GetBody().GetSyncToken()
	if next == resumeToken {
		next = ""
	}
	return PageResult{Items: items, NextToken: next}, nil
}

func (c *Client) ListRecent(ctx context.Context, n int) ([]MediaItem, error) {
	all := []MediaItem{}
	cursor := ""
	for {
		page, err := c.ListPage(ctx, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if page.NextToken == "" || (n > 0 && len(all) >= n*2) {
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
