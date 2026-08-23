package gpmc

import (
	"os"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// ExifDate returns the original capture date from an image file's EXIF metadata
// (DateTimeOriginal, falling back to DateTime). ok is false when there's no
// readable EXIF date — e.g. PNG, or HEIC (goexif reads JPEG/TIFF EXIF only), in
// which case callers keep the file's modification time.
func ExifDate(path string) (time.Time, bool) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		return time.Time{}, false
	}
	dt, err := x.DateTime()
	if err != nil {
		return time.Time{}, false
	}
	return dt, true
}
