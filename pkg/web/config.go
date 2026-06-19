package web

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Listen               string
	Username             string
	PasswordHash         string
	DeviceProfile        string
	TempDir              string
	MaxConcurrentUploads int
	SessionDays          int
	StreamTokenTTLMin    int
	SecretKey            []byte

	// S3-compatible gateway (optional). Enabled when S3Listen is non-empty.
	S3Listen    string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3Region    string

	// WebDAV gateway (optional). Enabled when WebDAVListen is non-empty.
	// Authenticates against Username/PasswordHash above.
	WebDAVListen   string
	WebDAVBasePath string
}

func LoadConfig(path string) (Config, error) {
	cfg := Config{
		Listen:               "0.0.0.0:8080",
		DeviceProfile:        "pixel-xl",
		MaxConcurrentUploads: 2,
		SessionDays:          30,
		StreamTokenTTLMin:    60,
		S3Bucket:             "gpix",
		S3Region:             "us-east-1",
		WebDAVBasePath:       "/",
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	for ln, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			return cfg, fmt.Errorf("config line %d: missing =", ln+1)
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)
		switch k {
		case "listen":
			cfg.Listen = v
		case "username":
			cfg.Username = v
		case "password_hash":
			cfg.PasswordHash = v
		case "device_profile":
			cfg.DeviceProfile = v
		case "temp_dir":
			cfg.TempDir = v
		case "max_concurrent_uploads":
			n, _ := strconv.Atoi(v)
			if n > 0 {
				cfg.MaxConcurrentUploads = n
			}
		case "session_days":
			n, _ := strconv.Atoi(v)
			if n > 0 {
				cfg.SessionDays = n
			}
		case "stream_token_ttl_minutes":
			n, _ := strconv.Atoi(v)
			if n > 0 {
				cfg.StreamTokenTTLMin = n
			}
		case "s3_listen":
			cfg.S3Listen = v
		case "s3_access_key":
			cfg.S3AccessKey = v
		case "s3_secret_key":
			cfg.S3SecretKey = v
		case "s3_bucket":
			if v != "" {
				cfg.S3Bucket = v
			}
		case "s3_region":
			if v != "" {
				cfg.S3Region = v
			}
		case "webdav_listen":
			cfg.WebDAVListen = v
		case "webdav_base_path":
			if v != "" {
				cfg.WebDAVBasePath = v
			}
		}
	}
	if cfg.Username == "" {
		return cfg, errors.New("config: username is required")
	}
	if cfg.PasswordHash == "" {
		return cfg, errors.New("config: password_hash is required")
	}
	if cfg.TempDir == "" {
		cfg.TempDir = os.TempDir()
	}
	return cfg, nil
}
