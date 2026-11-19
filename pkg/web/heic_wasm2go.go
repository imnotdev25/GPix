//go:build wasm2go

package web

// LeakyHEICDecoder: built with wasm2go, the HEIC decoder is plain Go and its
// memory is freed normally.
const LeakyHEICDecoder = false
