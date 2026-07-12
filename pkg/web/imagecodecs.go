package web

import (
	"image"

	"github.com/gen2brain/heic"
)

// Register HEIC/HEIF decoding so image.Decode (used by the thumbnail generator)
// can render encrypted HEIC photos — the Go standard library can't decode HEIC.
//
// github.com/gen2brain/heic is CGo-free (a Rust decoder compiled to WASM, run
// with wazero; it opportunistically uses a system libheif via purego when
// present). Its own init registers the "heic" brand; we additionally register
// the other common HEIF brands so photos from all devices decode.
func init() {
	for _, brand := range []string{"heix", "mif1", "msf1", "heim", "heis", "hevc", "heif"} {
		image.RegisterFormat("heic-"+brand, "????ftyp"+brand, heic.Decode, heic.DecodeConfig)
	}
}
