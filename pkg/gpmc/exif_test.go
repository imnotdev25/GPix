package gpmc

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type tiffEntry struct {
	tag    uint16
	s      string // ASCII value
	long   uint32 // LONG value, when isLong
	isLong bool
}

// buildTIFF lays out: header | IFD0 (+ Exif IFD pointer) | Exif IFD | strings.
func buildTIFF(o binary.ByteOrder, ifd0, exif []tiffEntry) []byte {
	ifdSize := func(n int) uint32 { return 2 + uint32(n)*12 + 4 }
	off0 := uint32(8)
	off1 := off0 + ifdSize(len(ifd0)+1)
	dataOff := off1 + ifdSize(len(exif))
	out := make([]byte, dataOff)
	if o == binary.ByteOrder(binary.LittleEndian) {
		copy(out, "II")
	} else {
		copy(out, "MM")
	}
	o.PutUint16(out[2:], 42)
	o.PutUint32(out[4:], off0)
	var data []byte
	put := func(at uint32, es []tiffEntry) {
		o.PutUint16(out[at:], uint16(len(es)))
		for i, e := range es {
			p := at + 2 + uint32(i)*12
			o.PutUint16(out[p:], e.tag)
			if e.isLong {
				o.PutUint16(out[p+2:], tiffLong)
				o.PutUint32(out[p+4:], 1)
				o.PutUint32(out[p+8:], e.long)
				continue
			}
			s := append([]byte(e.s), 0)
			o.PutUint16(out[p+2:], tiffASCII)
			o.PutUint32(out[p+4:], uint32(len(s)))
			if len(s) <= 4 {
				copy(out[p+8:], s)
				continue
			}
			o.PutUint32(out[p+8:], dataOff+uint32(len(data)))
			data = append(data, s...)
		}
	}
	put(off0, append(append([]tiffEntry{}, ifd0...), tiffEntry{tag: tagExifIFD, long: off1, isLong: true}))
	put(off1, exif)
	return append(out, data...)
}

func buildJPEG(tiff []byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xD8})
	app0 := []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")
	b.Write([]byte{0xFF, 0xE0})
	_ = binary.Write(&b, binary.BigEndian, uint16(len(app0)+2))
	b.Write(app0)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	b.Write([]byte{0xFF, 0xE1})
	_ = binary.Write(&b, binary.BigEndian, uint16(len(payload)+2))
	b.Write(payload)
	b.Write([]byte{0xFF, 0xDA, 0x00, 0x02})
	return b.Bytes()
}

func writeTemp(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExifDate(t *testing.T) {
	ist := time.FixedZone("", 5*3600+30*60)
	cases := []struct {
		name string
		file []byte
		want time.Time
		ok   bool
	}{
		{
			name: "jpeg original",
			file: buildJPEG(buildTIFF(binary.LittleEndian, nil,
				[]tiffEntry{{tag: tagDateTimeOriginal, s: "2021:07:04 10:20:30"}})),
			want: time.Date(2021, 7, 4, 10, 20, 30, 0, time.Local), ok: true,
		},
		{
			name: "tiff with offset",
			file: buildTIFF(binary.BigEndian, nil, []tiffEntry{
				{tag: tagDateTimeOriginal, s: "2019:01:02 03:04:05"},
				{tag: tagOffsetTimeOriginal, s: "+05:30"},
			}),
			want: time.Date(2019, 1, 2, 3, 4, 5, 0, ist), ok: true,
		},
		{
			name: "ifd0 fallback",
			file: buildJPEG(buildTIFF(binary.LittleEndian,
				[]tiffEntry{{tag: tagDateTime, s: "2015:05:06 07:08:09"}}, nil)),
			want: time.Date(2015, 5, 6, 7, 8, 9, 0, time.Local), ok: true,
		},
		{
			name: "blank date",
			file: buildJPEG(buildTIFF(binary.LittleEndian, nil,
				[]tiffEntry{{tag: tagDateTimeOriginal, s: "    :  :     :  :  "}})),
		},
		{
			name: "heic embedded",
			file: append(append([]byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0, 0, 0, 0, 'm', 'i', 'f', '1', 'h', 'e', 'i', 'c'},
				"Exif\x00\x00"...), buildTIFF(binary.BigEndian, nil,
				[]tiffEntry{{tag: tagDateTimeOriginal, s: "2023:12:24 18:00:00"}})...),
			want: time.Date(2023, 12, 24, 18, 0, 0, 0, time.Local), ok: true,
		},
		{name: "png", file: []byte("\x89PNG\r\n\x1a\n0000")},
	}
	for _, c := range cases {
		got, ok := ExifDate(writeTemp(t, "f", c.file))
		if ok != c.ok || (ok && !got.Equal(c.want)) {
			t.Errorf("%s: got %v %v, want %v %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

// Large files must not be read into memory: imports read several at once.
func TestExifDateReadsLittle(t *testing.T) {
	const size = 128 << 20
	tiff := buildTIFF(binary.LittleEndian, nil, []tiffEntry{{tag: tagDateTimeOriginal, s: "2020:02:02 02:02:02"}})
	cases := map[string][]byte{
		"raw":          tiff,
		"jpeg":         buildJPEG(tiff),
		"jpeg-no-exif": {0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10},
	}
	for name, head := range cases {
		p := filepath.Join(t.TempDir(), name)
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(head)
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		f.Close()

		runtime.GC()
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		_, ok := ExifDate(p)
		runtime.ReadMemStats(&b)
		if name != "jpeg-no-exif" && !ok {
			t.Errorf("%s: no date found", name)
		}
		if got := b.TotalAlloc - a.TotalAlloc; got > 1<<20 {
			t.Errorf("%s: allocated %d KB for a %d MB file", name, got>>10, size>>20)
		}
	}
}
