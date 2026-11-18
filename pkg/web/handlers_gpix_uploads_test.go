package web

import (
	"testing"

	"gpix/pkg/gpmc"
)

func TestSelectGpixUploads(t *testing.T) {
	items := []gpmc.MediaItem{
		{MediaKey: "phone1", Filename: "IMG_0001.jpg", Kind: gpmc.KindPhoto, SizeBytes: 100},
		{MediaKey: "phone2", Filename: "VID_0002.mp4", Kind: gpmc.KindVideo, SizeBytes: 200},
		{MediaKey: "gp1", Filename: "holiday.jpg", Kind: gpmc.KindPhoto, SizeBytes: 10},
		{MediaKey: "gp2", Filename: "clip.mov", Kind: gpmc.KindVideo, SizeBytes: 20},
		{MediaKey: "wrap1", Filename: "report.pdf.mp4", Kind: gpmc.KindVideo, SizeBytes: 30},
		{MediaKey: "wrap2", Filename: "notes.md.mp4", Kind: gpmc.KindVideo, SizeBytes: 40},
		{MediaKey: "wrap3", Filename: "secret.jpg.mp4", Kind: gpmc.KindVideo, SizeBytes: 50},
	}
	tracked := map[string]bool{"gp1": true, "gp2": true, "wrap1": true, "gone": true}

	sel, sum := selectGpixUploads(items, tracked)

	got := map[string]bool{}
	for _, it := range sel {
		got[it.MediaKey] = true
	}
	for _, k := range []string{"gp1", "gp2", "wrap1", "wrap2", "wrap3"} {
		if !got[k] {
			t.Errorf("missing %s", k)
		}
	}
	if got["phone1"] || got["phone2"] {
		t.Errorf("selected a non-gpix item: %v", got)
	}

	want := gpixUploadSummary{
		Total: 5, Photos: 2, Videos: 1, Docs: 1, Files: 1, Bytes: 150,
		Tracked: 3, Disguised: 2,
	}
	if sum.Total != want.Total || sum.Photos != want.Photos || sum.Videos != want.Videos ||
		sum.Docs != want.Docs || sum.Files != want.Files || sum.Bytes != want.Bytes ||
		sum.Tracked != want.Tracked || sum.Disguised != want.Disguised {
		t.Fatalf("summary = %+v, want %+v", sum, want)
	}
	if len(sum.Sample) != 5 || sum.Sample[2] != "report.pdf" {
		t.Errorf("sample = %v", sum.Sample)
	}
}

func TestSelectGpixUploadsEmpty(t *testing.T) {
	sel, sum := selectGpixUploads([]gpmc.MediaItem{
		{MediaKey: "p", Filename: "IMG.jpg", Kind: gpmc.KindPhoto},
	}, nil)
	if len(sel) != 0 || sum.Total != 0 || sum.Sample == nil {
		t.Fatalf("sel=%v sum=%+v", sel, sum)
	}
}
