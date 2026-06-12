package gpmc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type StorageQuota struct {
	UsageBytes int64
	LimitBytes int64
	Unlimited  bool
}

type driveTokenCache struct {
	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (c *Client) GetStorageQuota(ctx context.Context) (StorageQuota, error) {
	tok, err := c.driveBearer(ctx)
	if err != nil {
		return StorageQuota{}, fmt.Errorf("gpmc storage: get drive token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointDriveAbout, nil)
	if err != nil {
		return StorageQuota{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return StorageQuota{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return StorageQuota{}, &APIError{Op: "drive-about", Status: resp.StatusCode, Body: string(body)}
	}

	var parsed struct {
		StorageQuota struct {
			Limit string `json:"limit"`
			Usage string `json:"usage"`
		} `json:"storageQuota"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return StorageQuota{}, fmt.Errorf("gpmc storage: decode json: %w", err)
	}
	q := StorageQuota{}
	if v, err := strconv.ParseInt(parsed.StorageQuota.Usage, 10, 64); err == nil {
		q.UsageBytes = v
	}
	if parsed.StorageQuota.Limit == "" {
		q.Unlimited = true
	} else if v, err := strconv.ParseInt(parsed.StorageQuota.Limit, 10, 64); err == nil {
		q.LimitBytes = v
	}
	return q, nil
}

func (c *Client) driveBearer(ctx context.Context) (string, error) {
	c.drive.mu.Lock()
	defer c.drive.mu.Unlock()
	if c.drive.token != "" && time.Now().Add(30*time.Second).Before(c.drive.expiry) {
		return c.drive.token, nil
	}
	tok, exp, err := c.fetchScopedToken(ctx, "oauth2:https://www.googleapis.com/auth/drive.appdata")
	if err != nil {
		return "", err
	}
	c.drive.token, c.drive.expiry = tok, exp
	return tok, nil
}

func (c *Client) fetchScopedToken(ctx context.Context, service string) (string, time.Time, error) {
	parsed, err := url.ParseQuery(c.authData)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("gpmc: parse auth_data: %w", err)
	}
	form := url.Values{}
	for _, k := range authFields {
		if k == "service" {
			continue
		}
		if v := parsed.Get(k); v != "" {
			form.Set(k, v)
		}
	}
	form.Set("service", service)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointAuth, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", contentTypeForm)
	req.Header.Set("User-Agent", authUA(c.profile))
	req.Header.Set("app", "com.google.android.apps.photos")
	if dev := parsed.Get("androidId"); dev != "" {
		req.Header.Set("device", dev)
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, &AuthError{Status: resp.StatusCode, Body: string(body)}
	}
	kv := parseKVLines(string(body))
	tok := kv["Auth"]
	if tok == "" {
		return "", time.Time{}, &AuthError{Status: resp.StatusCode, Body: string(body)}
	}
	expSec, _ := strconv.ParseInt(kv["Expiry"], 10, 64)
	return tok, time.Unix(expSec, 0), nil
}
