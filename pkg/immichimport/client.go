// Package immichimport pulls assets out of an external Immich server and
// uploads them into gpix. gpix acts as a CLIENT here (authenticating with an
// Immich API key), the opposite of the archived Immich-compatible server.
//
// Enumeration uses the timeline-bucket API (what Immich's own web UI uses to
// list the whole library) rather than search pagination, which is unreliable
// across Immich versions and was capping imports at a single page.
package immichimport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to an Immich server's REST API using an API key.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// Asset is the subset of an Immich asset gpix needs to copy it.
type Asset struct {
	ID               string `json:"id"`
	Type             string `json:"type"` // IMAGE | VIDEO | OTHER
	OriginalFileName string `json:"originalFileName"`
	Checksum         string `json:"checksum"`
}

// Bucket is a timeline time-bucket with its asset count.
type Bucket struct {
	TimeBucket string `json:"timeBucket"`
	Count      int    `json:"count"`
}

// NewClient builds a client. baseURL is the Immich server root (e.g.
// https://immich.example.com); a trailing "/api" is tolerated.
func NewClient(baseURL, apiKey string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	baseURL = strings.TrimSuffix(baseURL, "/api")
	return &Client{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: 0}, // downloads can be large; rely on ctx
	}
}

func (c *Client) req(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// Ping verifies the server is reachable and the API key works.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := c.req(ctx, http.MethodGet, "/api/users/me", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("authentication failed (check the API key)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from Immich", resp.StatusCode)
	}
	return nil
}

// Buckets lists all timeline buckets (month granularity).
func (c *Client) Buckets(ctx context.Context) ([]Bucket, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := c.req(ctx, http.MethodGet, "/api/timeline/buckets?size=MONTH", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list timeline buckets: status %d", resp.StatusCode)
	}
	var buckets []Bucket
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&buckets); err != nil {
		return nil, err
	}
	return buckets, nil
}

// BucketAssets returns every asset in one timeline bucket. It accepts both the
// legacy array response and the newer columnar response.
func (c *Client) BucketAssets(ctx context.Context, timeBucket string) ([]Asset, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	q := url.Values{"size": {"MONTH"}, "timeBucket": {timeBucket}}
	req, err := c.req(ctx, http.MethodGet, "/api/timeline/bucket?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("timeline bucket %s: status %d", timeBucket, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' { // legacy: array of asset objects
		var arr []Asset
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}
	// columnar: parallel arrays keyed by field
	var col struct {
		ID      []string `json:"id"`
		IsImage []bool   `json:"isImage"`
	}
	if err := json.Unmarshal(trimmed, &col); err != nil {
		return nil, err
	}
	out := make([]Asset, len(col.ID))
	for i, id := range col.ID {
		a := Asset{ID: id, Type: "IMAGE"}
		if i < len(col.IsImage) && !col.IsImage[i] {
			a.Type = "VIDEO"
		}
		out[i] = a
	}
	return out, nil
}

// Download opens the original bytes for an asset and reports the original
// filename (from Content-Disposition) when the server provides it. The caller
// must Close the reader.
func (c *Client) Download(ctx context.Context, id string) (io.ReadCloser, string, error) {
	req, err := c.req(ctx, http.MethodGet, "/api/assets/"+id+"/original", nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("download %s: status %d", id, resp.StatusCode)
	}
	name := ""
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, perr := mime.ParseMediaType(cd); perr == nil {
			name = params["filename"]
		}
	}
	return resp.Body, name, nil
}
