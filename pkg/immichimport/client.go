// Package immichimport pulls assets out of an external Immich server and
// uploads them into gpix. gpix acts as a CLIENT here (authenticating with an
// Immich API key), the opposite of the archived Immich-compatible server.
package immichimport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Checksum         string `json:"checksum"` // base64 SHA-1 of the original
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
	// /api/users/me requires a valid key, so it validates auth (unlike ping).
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

// PageSize is how many assets are requested per search page.
const PageSize = 250

type searchResponse struct {
	Assets struct {
		Total int     `json:"total"`
		Items []Asset `json:"items"`
	} `json:"assets"`
}

// SearchPage returns one page of assets (1-based) and the reported total. The
// caller paginates by requesting successive pages until a page returns fewer
// than PageSize items — this is robust across Immich versions (the response's
// nextPage field is unreliable on some servers, which capped imports at one
// page).
func (c *Client) SearchPage(ctx context.Context, page int) (items []Asset, total int, err error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"page": page, "size": PageSize, "withExif": false})
	req, err := c.req(ctx, http.MethodPost, "/api/search/metadata", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("search assets: status %d", resp.StatusCode)
	}
	var sr searchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&sr); err != nil {
		return nil, 0, err
	}
	return sr.Assets.Items, sr.Assets.Total, nil
}

// Download opens the original bytes for an asset. The caller must Close it.
func (c *Client) Download(ctx context.Context, id string) (io.ReadCloser, error) {
	req, err := c.req(ctx, http.MethodGet, "/api/assets/"+id+"/original", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: status %d", id, resp.StatusCode)
	}
	return resp.Body, nil
}
