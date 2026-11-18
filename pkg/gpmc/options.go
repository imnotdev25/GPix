package gpmc

import (
	"net/http"
	"time"
)

type Quality int

const (
	QualityOriginal Quality = iota
	QualitySaver
	QualityUseQuota
)

type UploadOpts struct {
	Quality      Quality
	Force        bool
	Concurrency  int
	Recursive    bool
	DeleteAfter  bool
	OverrideName string
	// Mtime, when non-zero, sets the item's timestamp (e.g. the photo's capture
	// date from EXIF) instead of the source file's modification time.
	Mtime time.Time
}

type Option func(*Client)

func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpc = h }
}

func WithDeviceProfile(p DeviceProfile) Option {
	return func(c *Client) { c.profile = p }
}

func WithLanguage(lang string) Option {
	return func(c *Client) { c.language = lang }
}

func WithProxy(proxy string) Option {
	return func(c *Client) { c.proxy = proxy }
}

// UploadHook is called after gpix commits a new item to the library. It is not
// called when an upload is skipped because an identical item already exists,
// since that item may have been uploaded by another app.
type UploadHook func(item MediaItem)

// WithUploadHook registers a callback that runs after every committed upload.
func WithUploadHook(h UploadHook) Option {
	return func(c *Client) { c.onUpload = h }
}
