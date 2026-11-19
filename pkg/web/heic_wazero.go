//go:build !wasm2go

package web

// LeakyHEICDecoder is true when the binary was built without the wasm2go tag.
// The HEIC decoder then runs on wazero, which never frees a decoder instance:
// each large photo it renders leaves about 400 MB behind. Build with
// -tags "nodynamic wasm2go" (the Dockerfile and release builds do).
const LeakyHEICDecoder = true
