package disguise

import (
	"bytes"
	"mime"
	"path/filepath"
	"strings"
)

var mediaMIMEs = map[string]bool{
	"image/jpeg":        true,
	"image/png":         true,
	"image/heic":        true,
	"image/heif":        true,
	"image/webp":        true,
	"image/gif":         true,
	"image/avif":        true,
	"video/mp4":         true,
	"video/quicktime":   true,
	"video/webm":        true,
	"video/x-matroska":  true,
	"video/x-msvideo":   true,
	"video/3gpp":        true,
	"application/mp4":   true,
}

func IsMediaMIME(m string) bool {
	m = strings.ToLower(strings.TrimSpace(m))
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	return mediaMIMEs[m]
}

func IsMediaFile(head []byte) bool {
	switch {
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return true
	case bytes.HasPrefix(head, []byte{0x89, 0x50, 0x4E, 0x47}):
		return true
	case bytes.HasPrefix(head, []byte("GIF87a")) || bytes.HasPrefix(head, []byte("GIF89a")):
		return true
	case len(head) >= 12 && bytes.HasPrefix(head[8:], []byte("WEBP")):
		return true
	case len(head) >= 8 && bytes.Contains(head[:8], []byte("ftyp")):
		return true
	case bytes.HasPrefix(head, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return true
	}
	return false
}

func MIMEForFilename(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if m := mime.TypeByExtension(ext); m != "" {
		return m
	}
	return "application/octet-stream"
}

// LooksLikeDisguisedFilename heuristically detects a wrapped-as-mp4 filename
// by looking for a meaningful inner extension before the trailing ".mp4".
// e.g. "report.pdf.mp4" → true, "holiday.mp4" → false.
// Returns the inferred original filename (without trailing ".mp4") when matched.
func LooksLikeDisguisedFilename(name string) (origName string, ok bool) {
	if !strings.HasSuffix(strings.ToLower(name), ".mp4") {
		return "", false
	}
	stem := name[:len(name)-4]
	innerExt := strings.ToLower(filepath.Ext(stem))
	if innerExt == "" || innerExt == ".mp4" || innerExt == ".m4v" {
		return "", false
	}
	if mime.TypeByExtension(innerExt) == "" && !knownNonMimeExt[innerExt] {
		return "", false
	}
	return stem, true
}

var knownNonMimeExt = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dmg": true,
	".iso": true, ".bin": true, ".dat": true, ".log": true,
	".sql": true, ".lock": true, ".env": true, ".toml": true,
	".yaml": true, ".yml": true, ".ini": true, ".conf": true,
	".cfg": true, ".key": true, ".pem": true, ".crt": true,
	".7z": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true, ".rar": true,
	".apk": true, ".ipa": true, ".deb": true, ".rpm": true,
	".sh": true, ".bat": true, ".ps1": true, ".cmd": true,
	".md": true, ".rst": true, ".txt": true,
	".go": true, ".rs": true, ".py": true, ".js": true, ".ts": true,
	".c": true, ".cpp": true, ".h": true, ".java": true,
}

func ShouldWrap(declaredMIME, filename string, head []byte) bool {
	if declaredMIME != "" && IsMediaMIME(declaredMIME) {
		return false
	}
	if filename != "" {
		if guess := MIMEForFilename(filename); IsMediaMIME(guess) {
			return false
		}
	}
	if len(head) > 0 && IsMediaFile(head) {
		return false
	}
	return true
}
