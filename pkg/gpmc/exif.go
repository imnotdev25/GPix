package gpmc

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"
	"time"
)

// ExifDate returns when a photo or video was taken, read from the file itself:
// EXIF DateTimeOriginal for JPEG, TIFF/RAW and HEIC/HEIF/AVIF, or the movie
// header's creation time for MP4/MOV. ok is false when the file has no such
// date (PNG screenshots, for example); callers then fall back to another date.
//
// EXIF times use the camera's OffsetTimeOriginal when it wrote one; otherwise
// they're read in the server's local zone (set TZ to match where your photos
// are taken).
//
// It reads only the bytes it needs (box and directory headers and the date
// strings), never the whole file: RAW photos run to tens of megabytes, and an
// import reads several at once.
func ExifDate(path string) (time.Time, bool) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return time.Time{}, false
	}
	size := st.Size()

	head := make([]byte, 12)
	n, _ := f.ReadAt(head, 0)
	head = head[:n]
	switch {
	case isISOBMFF(head): // MP4, MOV, HEIC, HEIF, AVIF
		if t, ok := movieCreated(f); ok {
			return t, true
		}
		return embeddedExifDate(f)
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8}): // JPEG
		return jpegExifDate(f, size)
	case bytes.HasPrefix(head, []byte("II*\x00")), bytes.HasPrefix(head, []byte("MM\x00*")): // TIFF, DNG, most RAW
		return tiffDate(f, 0, size)
	}
	return time.Time{}, false
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

// readFull reads exactly n bytes at off, or reports false.
func readFull(r io.ReaderAt, off int64, n int) ([]byte, bool) {
	if off < 0 || n < 0 {
		return nil, false
	}
	b := make([]byte, n)
	got, _ := r.ReadAt(b, off)
	return b, got == n
}

// jpegExifDate walks a JPEG's marker segments (headers only) to the APP1 Exif
// segment and reads the date from it. Metadata comes before the image data,
// so it stops at the start of the scan.
func jpegExifDate(r io.ReaderAt, size int64) (time.Time, bool) {
	pos := int64(2) // after SOI
	for seg := 0; seg < 64 && pos+4 <= size; seg++ {
		hdr, ok := readFull(r, pos, 4)
		if !ok || hdr[0] != 0xFF {
			return time.Time{}, false
		}
		marker := hdr[1]
		switch {
		case marker == 0xFF: // fill byte
			pos++
			continue
		case marker == 0x01 || marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7): // no length
			pos += 2
			continue
		case marker == 0xDA || marker == 0xD9: // start of scan / end of image
			return time.Time{}, false
		}
		segLen := int64(binary.BigEndian.Uint16(hdr[2:4]))
		if segLen < 2 {
			return time.Time{}, false
		}
		if marker == 0xE1 && segLen >= 8 {
			if id, ok := readFull(r, pos+4, 6); ok && string(id) == "Exif\x00\x00" {
				end := pos + 2 + segLen
				if end > size {
					end = size
				}
				if t, ok := tiffDate(r, pos+10, end); ok {
					return t, true
				}
			}
		}
		pos += 2 + segLen
	}
	return time.Time{}, false
}

// embeddedExifDate finds the Exif block inside a HEIC/HEIF/AVIF container and
// reads the date from it.
func embeddedExifDate(f *os.File) (time.Time, bool) {
	buf := make([]byte, 4<<20) // the metadata sits near the start
	n, _ := f.ReadAt(buf, 0)
	buf = buf[:n]
	r := bytes.NewReader(buf)
	marker := []byte("Exif\x00\x00")
	for off := 0; ; {
		i := bytes.Index(buf[off:], marker)
		if i < 0 {
			return time.Time{}, false
		}
		start := off + i + len(marker)
		if t, ok := tiffDate(r, int64(start), int64(len(buf))); ok {
			return t, true
		}
		off = start
	}
}

// ---- TIFF / EXIF directories ---------------------------------------------------

const (
	tagDateTime            = 0x0132 // IFD0
	tagExifIFD             = 0x8769 // IFD0: offset of the Exif IFD
	tagDateTimeOriginal    = 0x9003
	tagDateTimeDigitized   = 0x9004
	tagOffsetTime          = 0x9010
	tagOffsetTimeOriginal  = 0x9011
	tagOffsetTimeDigitized = 0x9012

	tiffASCII = 2
	tiffLong  = 4
	tiffIFD   = 13
)

type ifdEntry struct {
	typ   uint16
	count uint32
	val   [4]byte // the value itself when it fits, otherwise its offset
}

// tiffDir reads TIFF structures through r. Offsets inside the structure are
// relative to base (where the TIFF header starts), and no read goes past limit.
type tiffDir struct {
	r     io.ReaderAt
	base  int64
	limit int64
	order binary.ByteOrder
}

func (t *tiffDir) read(off int64, n int) ([]byte, bool) {
	if off < 0 || t.base+off+int64(n) > t.limit {
		return nil, false
	}
	return readFull(t.r, t.base+off, n)
}

// ifd reads one image file directory: a count, then 12-byte entries.
func (t *tiffDir) ifd(off int64) (map[uint16]ifdEntry, bool) {
	h, ok := t.read(off, 2)
	if !ok {
		return nil, false
	}
	n := int(t.order.Uint16(h))
	if n == 0 || n > 1000 {
		return nil, false
	}
	b, ok := t.read(off+2, n*12)
	if !ok {
		return nil, false
	}
	m := make(map[uint16]ifdEntry, n)
	for i := 0; i < n; i++ {
		e := b[i*12 : i*12+12]
		var ent ifdEntry
		ent.typ = t.order.Uint16(e[2:4])
		ent.count = t.order.Uint32(e[4:8])
		copy(ent.val[:], e[8:12])
		m[t.order.Uint16(e[0:2])] = ent
	}
	return m, true
}

func (t *tiffDir) ascii(e ifdEntry) (string, bool) {
	if e.typ != tiffASCII || e.count == 0 || e.count > 64 {
		return "", false
	}
	b := e.val[:]
	if e.count <= 4 {
		b = b[:e.count]
	} else {
		var ok bool
		if b, ok = t.read(int64(t.order.Uint32(e.val[:])), int(e.count)); !ok {
			return "", false
		}
	}
	return strings.TrimRight(string(b), "\x00 "), true
}

// date reads the date in tag dateTag of dir, in the zone given by zoneTag of
// zoneDir when present.
func (t *tiffDir) date(dir map[uint16]ifdEntry, dateTag uint16, zoneDir map[uint16]ifdEntry, zoneTag uint16) (time.Time, bool) {
	e, ok := dir[dateTag]
	if !ok {
		return time.Time{}, false
	}
	s, ok := t.ascii(e)
	if !ok {
		return time.Time{}, false
	}
	loc := time.Local
	if z, ok := zoneDir[zoneTag]; ok {
		if zs, ok := t.ascii(z); ok {
			if zt, err := time.Parse("Z07:00", zs); err == nil {
				_, off := zt.Zone()
				loc = time.FixedZone("", off)
			}
		}
	}
	for _, layout := range []string{"2006:01:02 15:04:05", "2006-01-02 15:04:05"} {
		if d, err := time.ParseInLocation(layout, s, loc); err == nil {
			return d, plausibleDate(d)
		}
	}
	return time.Time{}, false
}

// tiffDate reads DateTimeOriginal (then DateTimeDigitized, then IFD0's
// DateTime) from the TIFF structure that starts at base in r.
func tiffDate(r io.ReaderAt, base, limit int64) (time.Time, bool) {
	t := &tiffDir{r: r, base: base, limit: limit}
	h, ok := t.read(0, 8)
	if !ok {
		return time.Time{}, false
	}
	switch string(h[:2]) {
	case "II":
		t.order = binary.LittleEndian
	case "MM":
		t.order = binary.BigEndian
	default:
		return time.Time{}, false
	}
	if t.order.Uint16(h[2:4]) != 42 {
		return time.Time{}, false
	}
	ifd0, ok := t.ifd(int64(t.order.Uint32(h[4:8])))
	if !ok {
		return time.Time{}, false
	}
	var exif map[uint16]ifdEntry
	if e, ok := ifd0[tagExifIFD]; ok && (e.typ == tiffLong || e.typ == tiffIFD) && e.count == 1 {
		exif, _ = t.ifd(int64(t.order.Uint32(e.val[:])))
	}
	if d, ok := t.date(exif, tagDateTimeOriginal, exif, tagOffsetTimeOriginal); ok {
		return d, true
	}
	if d, ok := t.date(exif, tagDateTimeDigitized, exif, tagOffsetTimeDigitized); ok {
		return d, true
	}
	return t.date(ifd0, tagDateTime, exif, tagOffsetTime)
}

// ---- MP4 / MOV ---------------------------------------------------------------------

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
