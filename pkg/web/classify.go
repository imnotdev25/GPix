package web

import (
	"path/filepath"
	"strings"

	"gpix/pkg/disguise"
	"gpix/pkg/gpmc"
)

// mediaClass is how an item is presented in the gpix UI.
type mediaClass string

const (
	classPhoto mediaClass = "photo"
	classVideo mediaClass = "video"
	classDoc   mediaClass = "doc"
	classFile  mediaClass = "file"
)

// textExts are document/text formats gpix renders to a readable image while
// keeping the original downloadable.
var textExts = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".text": true, ".log": true,
	".csv": true, ".tsv": true, ".json": true, ".yaml": true, ".yml": true,
	".ini": true, ".conf": true, ".cfg": true, ".toml": true, ".xml": true,
	".html": true, ".htm": true, ".css": true, ".js": true, ".ts": true,
	".go": true, ".py": true, ".rb": true, ".rs": true, ".c": true, ".cpp": true,
	".h": true, ".java": true, ".sh": true, ".sql": true, ".env": true, ".rst": true,
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".heic": true, ".heif": true, ".avif": true, ".bmp": true, ".tiff": true, ".tif": true,
	".dng": true,
}

var videoExts = map[string]bool{
	".mov": true, ".m4v": true, ".webm": true, ".mkv": true, ".avi": true,
	".3gp": true, ".3gpp": true, ".hevc": true, ".mpg": true, ".mpeg": true, ".wmv": true, ".flv": true,
}

// classifyItem decides how to present an item. Disguised items (including
// encrypted ones) are presented by their original/inner extension, so encrypted
// photos show as photos and encrypted videos as videos — gpix decrypts them on
// the fly. Genuine disguised non-media files stay as file cards.
//
// Heuristic limit: a disguised item is detected from its filename, and the
// detector can't see through a doubled ".mp4"/".m4v" inner extension, so an
// encrypted video whose original name ends in .mp4 falls back to the normal
// (Google-side) video path. Encrypted photos and non-.mp4 videos are fine.
func classifyItem(filename string, kind gpmc.MediaKind) (display string, class mediaClass, disguised bool) {
	if orig, ok := disguise.LooksLikeDisguisedFilename(filename); ok {
		ext := strings.ToLower(filepath.Ext(orig))
		switch {
		case imageExts[ext]:
			return orig, classPhoto, true
		case videoExts[ext]:
			return orig, classVideo, true
		case textExts[ext]:
			return orig, classDoc, true
		default:
			return orig, classFile, true
		}
	}
	if kind == gpmc.KindVideo {
		return filename, classVideo, false
	}
	return filename, classPhoto, false
}

// classForExt classifies a display name (no disguise ".mp4") purely by its
// extension. Used when a client hints the name so thumbnailing doesn't depend
// on the library cache being warm.
func classForExt(name string) mediaClass {
	switch ext := strings.ToLower(filepath.Ext(name)); {
	case imageExts[ext]:
		return classPhoto
	case videoExts[ext]:
		return classVideo
	case textExts[ext]:
		return classDoc
	default:
		return classFile
	}
}

// canGenerateThumb reports whether gpix can build a thumbnail for a decrypted
// photo. JPEG/PNG/GIF are stdlib; HEIC/HEIF via the CGo-free decoder registered
// in imagecodecs.go. Other formats (WebP, RAW) fall back to the Google-side
// thumbnail, which is blank for encrypted items.
func canGenerateThumb(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".heic", ".heif":
		return true
	}
	return false
}
