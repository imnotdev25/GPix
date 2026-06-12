package bridge

import "strings"

type SendMode int

const (
	SendAsPhoto SendMode = iota
	SendAsVideo
	SendAsDocument
)

func ResolveSendMode(mimeType string) SendMode {
	m := strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	switch {
	case m == "image/jpeg", m == "image/png", m == "image/webp", m == "image/gif":
		return SendAsPhoto
	case strings.HasPrefix(m, "video/"):
		return SendAsVideo
	default:
		return SendAsDocument
	}
}
