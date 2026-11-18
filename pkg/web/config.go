package web

import (
	"os"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	Listen               string
	DeviceProfile        string
	TempDir              string
	MaxConcurrentUploads int
	SessionDays          int
	StreamTokenTTLMin    int
	SecretKey            []byte

	// LegacyUsername / LegacyPasswordHash hold the old GPIX_USERNAME and
	// GPIX_PASSWORD_HASH settings. They're optional: when both are set, gpix
	// imports them once as a sign-in account so existing setups keep working.
	LegacyUsername     string
	LegacyPasswordHash string

	// DataDir is where runtime state lives (secret.key, gateways.json,
	// encryption.key/json, *.db, the thumbnail cache). Set via GPIX_DATA_DIR;
	// defaults to the working directory.
	DataDir string

	// S3-compatible gateway (optional). Enabled when S3Listen is non-empty.
	S3Listen    string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3Region    string

	// WebDAV gateway (optional). Enabled when WebDAVListen is non-empty.
	// Accepts a gpix account's email + password, or the WebDAV app password.
	WebDAVListen   string
	WebDAVBasePath string

	// EncryptUploads seeds the "encrypt new uploads" toggle on first run (before
	// the user flips it in the web UI). Once set via the UI, that choice wins.
	EncryptUploads bool

	// ThumbGen lets gpix render thumbnails itself for encrypted/disguised items
	// (Google only has a blank one for those). On by default; GPIX_THUMB_GEN=false
	// turns it off to save CPU, and already-rendered thumbnails are still served.
	ThumbGen bool

	// ServerURL is the externally-reachable base URL of this gpix instance
	// (e.g. https://photos.example.com), used to build absolute share links and
	// redirects. Falls back to the request's own host when empty.
	ServerURL string

	// Logto / OIDC login (optional). Enabled when LogtoEndpoint, LogtoClientID
	// and LogtoClientSecret are all set.
	LogtoEndpoint     string
	LogtoClientID     string
	LogtoClientSecret string
	LogtoRedirect     string // defaults to ServerURL + /auth/logto/callback

	// SignupAllowlist (emails or @domains) limits who may create an account,
	// by password sign-up or single sign-on. Empty allows anyone.
	SignupAllowlist []string
	// MaxUsers caps how many accounts can exist (0 = no cap). The default, 1,
	// means the first person to sign up owns the server and sign-up then closes.
	MaxUsers int

	// DisablePasswordLogin hides the email/password form and rejects password
	// logins, leaving only OIDC. Ignored unless Logto is enabled, so you can't
	// lock yourself out by disabling both.
	DisablePasswordLogin bool
}

// LogtoEnabled reports whether OIDC login is configured.
func (c Config) LogtoEnabled() bool {
	return c.LogtoEndpoint != "" && c.LogtoClientID != "" && c.LogtoClientSecret != ""
}

// PasswordLoginEnabled reports whether email/password sign-in is active.
// Password login can only be turned off when OIDC is available.
func (c Config) PasswordLoginEnabled() bool {
	return !(c.DisablePasswordLogin && c.LogtoEnabled())
}

// LoadConfig builds the configuration from environment variables (typically
// loaded from .env). There is no config file.
func LoadConfig() Config {
	cfg := Config{
		Listen:               "0.0.0.0:8080",
		DeviceProfile:        "pixel-xl",
		MaxConcurrentUploads: 2,
		SessionDays:          30,
		StreamTokenTTLMin:    60,
		S3Bucket:             "gpix",
		S3Region:             "us-east-1",
		WebDAVBasePath:       "/",
		MaxUsers:             1,
		ThumbGen:             true,
	}
	applyEnv(&cfg)
	if cfg.TempDir == "" {
		cfg.TempDir = os.TempDir()
	}
	return cfg
}

// LegacyConfVars reads an old gpix-web.conf (no longer used) and returns the
// environment variable names for the settings it contains, so startup can tell
// the user what to move into .env. found is false when there's no such file.
func LegacyConfVars(path string) (vars []string, found bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		if name, ok := envNames[strings.TrimSpace(line[:eq])]; ok && !seen[name] {
			seen[name] = true
			vars = append(vars, name)
		}
	}
	sort.Strings(vars)
	return vars, true
}

// applyKey sets a single config key from its (already unquoted) string value.
func applyKey(cfg *Config, k, v string) {
	switch k {
	case "listen":
		cfg.Listen = v
	case "username":
		cfg.LegacyUsername = v
	case "password_hash":
		cfg.LegacyPasswordHash = v
	case "device_profile":
		cfg.DeviceProfile = v
	case "temp_dir":
		cfg.TempDir = v
	case "data_dir":
		cfg.DataDir = v
	case "max_concurrent_uploads":
		if n, _ := strconv.Atoi(v); n > 0 {
			cfg.MaxConcurrentUploads = n
		}
	case "session_days":
		if n, _ := strconv.Atoi(v); n > 0 {
			cfg.SessionDays = n
		}
	case "stream_token_ttl_minutes":
		if n, _ := strconv.Atoi(v); n > 0 {
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
	case "encrypt_uploads":
		cfg.EncryptUploads = truthy(v)
	case "thumb_gen":
		if strings.TrimSpace(v) != "" {
			cfg.ThumbGen = truthy(v)
		}
	case "server_url":
		cfg.ServerURL = strings.TrimRight(v, "/")
	case "logto_endpoint":
		cfg.LogtoEndpoint = strings.TrimRight(v, "/")
	case "logto_client_id":
		cfg.LogtoClientID = v
	case "logto_client_secret":
		cfg.LogtoClientSecret = v
	case "logto_redirect":
		cfg.LogtoRedirect = v
	case "signup_allowlist":
		cfg.SignupAllowlist = splitList(v)
	case "max_users":
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			cfg.MaxUsers = n
		}
	case "disable_password_login":
		cfg.DisablePasswordLogin = truthy(v)
	}
}

// splitList parses a comma- or space-separated list into trimmed entries.
func splitList(v string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// envNames maps each setting to its environment variable.
var envNames = map[string]string{
	"listen":                   "GPIX_LISTEN",
	"username":                 "GPIX_USERNAME",
	"password_hash":            "GPIX_PASSWORD_HASH",
	"device_profile":           "GPIX_DEVICE_PROFILE",
	"temp_dir":                 "GPIX_TEMP_DIR",
	"data_dir":                 "GPIX_DATA_DIR",
	"max_concurrent_uploads":   "GPIX_MAX_CONCURRENT_UPLOADS",
	"session_days":             "GPIX_SESSION_DAYS",
	"stream_token_ttl_minutes": "GPIX_STREAM_TOKEN_TTL_MIN",
	"s3_listen":                "GPIX_S3_LISTEN",
	"s3_access_key":            "GPIX_S3_ACCESS_KEY",
	"s3_secret_key":            "GPIX_S3_SECRET_KEY",
	"s3_bucket":                "GPIX_S3_BUCKET",
	"s3_region":                "GPIX_S3_REGION",
	"webdav_listen":            "GPIX_WEBDAV_LISTEN",
	"webdav_base_path":         "GPIX_WEBDAV_BASE_PATH",
	"encrypt_uploads":          "GPIX_ENCRYPT_UPLOADS",
	"thumb_gen":                "GPIX_THUMB_GEN",
	"server_url":               "GPIX_SERVER_URL",
	"logto_endpoint":           "GPIX_LOGTO_ENDPOINT",
	"logto_client_id":          "GPIX_LOGTO_CLIENT_ID",
	"logto_client_secret":      "GPIX_LOGTO_CLIENT_SECRET",
	"logto_redirect":           "GPIX_LOGTO_REDIRECT",
	"signup_allowlist":         "GPIX_SIGNUP_ALLOWLIST",
	"max_users":                "GPIX_MAX_USERS",
	"disable_password_login":   "GPIX_DISABLE_PASSWORD_LOGIN",
}

// applyEnv overlays GPIX_* environment variables (plus SERVER_URL) onto cfg.
func applyEnv(cfg *Config) {
	for key, envName := range envNames {
		if v, ok := os.LookupEnv(envName); ok {
			applyKey(cfg, key, v)
		}
	}
	// SERVER_URL (without the GPIX_ prefix) is the documented name and wins last.
	if v, ok := os.LookupEnv("SERVER_URL"); ok {
		cfg.ServerURL = strings.TrimRight(v, "/")
	}
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}
