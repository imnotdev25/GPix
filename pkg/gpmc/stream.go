package gpmc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

func (c *Client) GetStreamManifest(ctx context.Context, mediaKey, protocol string) (string, error) {
	if protocol == "" {
		protocol = "hls"
	}
	if protocol != "hls" && protocol != "dash" {
		return "", fmt.Errorf("gpmc stream: protocol must be hls or dash, got %q", protocol)
	}
	tok, err := c.bearer(ctx)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf(endpointStreamManifest, mediaKey, protocol)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
	if err != nil {
		return "", err
	}
	req.ContentLength = 0
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("User-Agent", apiUA(c.profile, c.language))

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", &APIError{Op: "stream-manifest", Status: resp.StatusCode, Body: string(body)}
	}
	return string(body), nil
}

func ThumbnailURL(mediaKey string, size int) string {
	if size <= 0 {
		size = 256
	}
	return fmt.Sprintf(endpointThumbnail, mediaKey, size, size)
}
