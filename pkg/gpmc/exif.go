package gpmc

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// ExifDate returns when a photo or video was taken, read from the file itself:
// EXIF DateTimeOriginal for JPEG, TIFF/RAW and HEIC/HEIF/AVIF, or the movie
// header's creation time for MP4/MOV. ok is false when the file has no such
// date (PNG screenshots, for example); callers then fall back to another date.
//
// EXIF times carry no time zone unless the camera wrote one; they're read in
// the server's local zone (set TZ to match where your photos are taken).
func ExifDate(path string) (time.Time, bool) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()

	head := make([]byte, 12)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	switch {
	case isISOBMFF(head): // MP4, MOV, HEIC, HEIF, AVIF
		if t, ok := movieCreated(f); ok {
			return t, true
		}
		return embeddedExifDate(f)
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8}), // JPEG
		bytes.HasPrefix(head, []byte("II*\x00")), bytes.HasPrefix(head, []byte("MM\x00*")): // TIFF, DNG, most RAW
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return time.Time{}, false
		}
		return decodeExifDate(f)
	}
	return time.Time{}, false
}

func decodeExifDate(r io.Reader) (time.Time, bool) {
	x, err := exif.Decode(r)
	if err != nil {
		return time.Time{}, false
	}
	t, err := x.DateTime()
	if err != nil || !plausibleDate(t) {
		return time.Time{}, false
	}
	return t, true
}

func plausibleDate(t time.Time) bool {
	return t.Year() >= 1971 && t.Before(time.Now().Add(48*time.Hour))
}

func isISOBMFF(head []byte) bool {
	if len(head) < 8 {
		return false
	}
	switch string(head[4:8]) {
	case "ftyp", "moov", "mdat", "wide", "free", "skip":
		return true
	}
	return false
}

// embeddedExifDate finds the Exif block inside a HEIC/HEIF/AVIF container
// (goexif only understands JPEG and TIFF framing) and decodes it.
func embeddedExifDate(f *os.File) (time.Time, bool) {
	buf := make([]byte, 4<<20) // the metadata sits near the start
	n, _ := f.ReadAt(buf, 0)
	buf = buf[:n]
	marker := []byte("Exif\x00\x00")
	for off := 0; ; {
		i := bytes.Index(buf[off:], marker)
		if i < 0 {
			return time.Time{}, false
		}
		start := off + i + len(marker)
		if t, ok := decodeExifDate(bytes.NewReader(buf[start:])); ok {
			return t, true
		}
		off = start
	}
}

// movieCreated reads creation_time from the moov/mvhd box of an MP4/MOV file.
func movieCreated(f *os.File) (time.Time, bool) {
	st, err := f.Stat()
	if err != nil {
		return time.Time{}, false
	}
	moov, size, ok := findBox(f, 0, st.Size(), "moov")
	if !ok {
		return time.Time{}, false
	}
	mvhd, _, ok := findBox(f, moov, moov+size, "mvhd")
	if !ok {
		return time.Time{}, false
	}
	hdr := make([]byte, 12)
	if _, err := f.ReadAt(hdr, mvhd); err != nil {
		return time.Time{}, false
	}
	var secs uint64
	if hdr[0] == 1 { // version 1: 64-bit times
		secs = binary.BigEndian.Uint64(hdr[4:12])
	} else {
		secs = uint64(binary.BigEndian.Uint32(hdr[4:8]))
	}
	const mac1904 = 2082844800 // seconds from 1904-01-01 to the Unix epoch
	if secs <= mac1904 {
		return time.Time{}, false
	}
	t := time.Unix(int64(secs-mac1904), 0)
	return t, plausibleDate(t)
}

// findBox scans the boxes between start and end for one of type typ and
// returns the offset of its payload and the payload size.
func findBox(f *os.File, start, end int64, typ string) (payload, size int64, ok bool) {
	hdr := make([]byte, 16)
	for pos := start; pos+8 <= end; {
		if _, err := f.ReadAt(hdr[:8], pos); err != nil {
			return 0, 0, false
		}
		boxSize := int64(binary.BigEndian.Uint32(hdr[:4]))
		hlen := int64(8)
		switch boxSize {
		case 1: // 64-bit size follows
			if _, err := f.ReadAt(hdr[8:16], pos+8); err != nil {
				return 0, 0, false
			}
			boxSize = int64(binary.BigEndian.Uint64(hdr[8:16]))
			hlen = 16
		case 0: // runs to the end
			boxSize = end - pos
		}
		if boxSize < hlen {
			return 0, 0, false
		}
		if string(hdr[4:8]) == typ {
			return pos + hlen, boxSize - hlen, true
		}
		pos += boxSize
	}
	return 0, 0, false
}
