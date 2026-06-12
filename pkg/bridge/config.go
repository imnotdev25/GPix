package bridge

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	BotToken      string
	APIID         int32
	APIHash       string
	OwnerID       int64
	SessionFile   string
	TempDir       string
	MaxConcurrent int
}

func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		BotToken:    os.Getenv("TG_BOT_TOKEN"),
		APIHash:     os.Getenv("TG_API_HASH"),
		SessionFile: getenvDefault("TG_SESSION_FILE", "gpixbot.session"),
		TempDir:     getenvDefault("TG_TEMP_DIR", os.TempDir()),
	}
	if cfg.BotToken == "" {
		return cfg, errors.New("TG_BOT_TOKEN is required")
	}
	if cfg.APIHash == "" {
		return cfg, errors.New("TG_API_HASH is required")
	}
	apiID, err := strconv.ParseInt(os.Getenv("TG_API_ID"), 10, 32)
	if err != nil || apiID == 0 {
		return cfg, errors.New("TG_API_ID must be a non-zero integer")
	}
	cfg.APIID = int32(apiID)

	owner, err := strconv.ParseInt(os.Getenv("TG_OWNER_ID"), 10, 64)
	if err != nil || owner == 0 {
		return cfg, errors.New("TG_OWNER_ID must be a non-zero integer (your Telegram user id)")
	}
	cfg.OwnerID = owner

	cfg.MaxConcurrent = 2
	if v := os.Getenv("TG_MAX_CONCURRENT"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			cfg.MaxConcurrent = n
		}
	}
	return cfg, nil
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
