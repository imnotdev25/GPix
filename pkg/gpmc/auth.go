package gpmc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (c *Client) bearer(ctx context.Context) (string, error) {
	c.tokMu.Lock()
	defer c.tokMu.Unlock()
	if c.token != "" && time.Now().Add(30*time.Second).Before(c.expires) {
		return c.token, nil
	}
	tok, exp, err := c.fetchAuthToken(ctx)
	if err != nil {
		return "", err
	}
	c.token, c.expires = tok, exp
	return tok, nil
}

func (c *Client) invalidateToken() {
	c.tokMu.Lock()
	c.token = ""
	c.expires = time.Time{}
	c.tokMu.Unlock()
}

var authFields = []string{
	"androidId", "app", "client_sig", "callerPkg", "callerSig",
	"device_country", "Email", "google_play_services_version",
	"lang", "oauth2_foreground", "sdk_version", "service", "Token",
}

func (c *Client) fetchAuthToken(ctx context.Context) (string, time.Time, error) {
	parsed, err := url.ParseQuery(c.authData)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("gpmc: parse auth_data: %w", err)
	}
	form := url.Values{}
	for _, k := range authFields {
		if v := parsed.Get(k); v != "" {
			form.Set(k, v)
		}
	}
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
	exp := time.Unix(expSec, 0)
	return tok, exp, nil
}

func parseKVLines(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		out[line[:i]] = line[i+1:]
	}
	return out
}
