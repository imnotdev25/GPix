package gpmc

import (
	"path/filepath"
	"strings"
)

var videoExtensions = map[string]bool{
	".mp4": true, ".mov": true, ".m4v": true, ".webm": true, ".mkv": true, ".avi": true,
	".3gp": true, ".3gpp": true, ".mpg": true, ".mpeg": true, ".wmv": true, ".flv": true,
	".hevc": true, ".mts": true, ".m2ts": true,
}

// KindForName guesses whether Google will list an upload as a photo or a video
// from its committed filename, for showing it before the next listing.
func KindForName(name string) MediaKind {
	if videoExtensions[strings.ToLower(filepath.Ext(name))] {
		return KindVideo
	}
	return KindPhoto
}
