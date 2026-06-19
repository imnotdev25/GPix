// Package mediacrypt provides authenticated, streaming encryption for media
// before it is uploaded to Google Photos. The plaintext (a photo, video, or any
// file) is encrypted with a gpix-managed key and the resulting opaque blob is
// then disguised as an MP4 (see pkg/disguise) so Google accepts it. Google — and
// anyone else without the key — only ever sees a 1-second solid-colour video.
//
// Format (little of it is secret; the key is): a header followed by a sequence
// of AES-256-GCM chunks (the age/Tink "STREAM" construction). A fresh 256-bit
// content key is derived per file via HKDF-SHA256 over a random salt, so chunk
// nonces are just a counter. The header is authenticated as additional data on
// the first chunk, and the final chunk carries a distinct nonce flag, which
// together defend against tampering, reordering and truncation.
//
//	magic     "GPIXENC1"            (8)
//	version   1                     (1)
//	keyID                           (1)   reserved for future key rotation
//	chunkSize uint32 BE             (4)   plaintext bytes per chunk
//	origSize  uint64 BE             (8)   total plaintext length
//	saltLen   uint8 (=16)           (1)
//	salt                            (16)
//	nameLen   uint16 BE             (2)
//	name      UTF-8 original name   (nameLen)
//	-- then ceil(origSize/chunkSize) (>=1) GCM chunks --
package mediacrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	// Magic identifies an encrypted blob.
	Magic = "GPIXENC1"

	version      = 1
	tagSize      = 16
	saltSize     = 16
	keySize      = 32 // AES-256
	defaultChunk = 64 * 1024
	maxChunk     = 16 << 20 // sanity bound when decoding untrusted headers
	hkdfInfo     = "gpix-media-v1"
)

var (
	// ErrNotEncrypted is returned when the stream does not start with Magic.
	ErrNotEncrypted = errors.New("mediacrypt: not an encrypted stream")
	// ErrCorrupt indicates a malformed or tampered stream.
	ErrCorrupt = errors.New("mediacrypt: corrupt or tampered stream")
)

// Header is the parsed, authenticated metadata of an encrypted stream.
type Header struct {
	Version   int
	KeyID     byte
	ChunkSize int
	OrigSize  int64
	Name      string
}

// HasMagic reports whether b begins with the encrypted-stream magic.
func HasMagic(b []byte) bool {
	return len(b) >= len(Magic) && string(b[:len(Magic)]) == Magic
}

// EncryptedSize returns the exact length of the encrypted output for a plaintext
// of origSize bytes with the given name. Useful for setting sizes up front.
func EncryptedSize(origSize int64, name string) int64 {
	hdr := int64(len(Magic) + 1 + 1 + 4 + 8 + 1 + saltSize + 2 + len(name))
	chunks := numChunks(origSize, defaultChunk)
	return hdr + origSize + chunks*tagSize
}

func numChunks(origSize int64, chunk int) int64 {
	if origSize <= 0 {
		return 1
	}
	return (origSize + int64(chunk) - 1) / int64(chunk)
}

func deriveKey(master, salt []byte) ([]byte, error) {
	r := hkdf.New(sha256.New, master, salt, []byte(hkdfInfo))
	k := make([]byte, keySize)
	if _, err := io.ReadFull(r, k); err != nil {
		return nil, err
	}
	return k, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// makeNonce builds the 12-byte GCM nonce for chunk i. The final chunk sets the
// trailing flag byte so truncating the stream fails authentication.
func makeNonce(counter uint64, last bool) []byte {
	n := make([]byte, 12)
	binary.BigEndian.PutUint64(n[0:8], counter)
	if last {
		n[11] = 1
	}
	return n
}

// Encrypt writes the encrypted stream for exactly origSize bytes read from src
// to dst. name is stored (authenticated) in the header so the original filename
// survives the round-trip.
func Encrypt(dst io.Writer, src io.Reader, master []byte, origSize int64, name string) error {
	if len(master) != keySize {
		return fmt.Errorf("mediacrypt: master key must be %d bytes", keySize)
	}
	if origSize < 0 {
		return errors.New("mediacrypt: negative size")
	}
	if len(name) > 0xFFFF {
		return errors.New("mediacrypt: name too long")
	}

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key, err := deriveKey(master, salt)
	if err != nil {
		return err
	}
	aead, err := newAEAD(key)
	if err != nil {
		return err
	}

	header := buildHeader(Header{Version: version, ChunkSize: defaultChunk, OrigSize: origSize, Name: name}, salt)
	if _, err := dst.Write(header); err != nil {
		return err
	}

	chunks := numChunks(origSize, defaultChunk)
	buf := make([]byte, defaultChunk)
	remaining := origSize
	for i := int64(0); i < chunks; i++ {
		ptLen := int64(defaultChunk)
		if remaining < ptLen {
			ptLen = remaining
		}
		if _, err := io.ReadFull(src, buf[:ptLen]); err != nil {
			return fmt.Errorf("mediacrypt: read plaintext: %w", err)
		}
		var aad []byte
		if i == 0 {
			aad = header
		}
		ct := aead.Seal(nil, makeNonce(uint64(i), i == chunks-1), buf[:ptLen], aad)
		if _, err := dst.Write(ct); err != nil {
			return err
		}
		remaining -= ptLen
	}
	return nil
}

// Decrypt reads an encrypted stream from src and writes the recovered plaintext
// to dst, returning the authenticated header.
func Decrypt(dst io.Writer, src io.Reader, master []byte) (Header, error) {
	hdr, headerBytes, salt, err := ParseHeader(src)
	if err != nil {
		return Header{}, err
	}
	if err := decryptBody(dst, src, master, hdr, headerBytes, salt); err != nil {
		return Header{}, err
	}
	return hdr, nil
}

// DecryptingReader parses the header synchronously (so the caller immediately
// knows the original name/size, or gets an error), then returns a reader that
// streams the decrypted body. Close the returned reader to release the
// background decryption goroutine.
func DecryptingReader(src io.Reader, master []byte) (Header, io.ReadCloser, error) {
	hdr, headerBytes, salt, err := ParseHeader(src)
	if err != nil {
		return Header{}, nil, err
	}
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(decryptBody(pw, src, master, hdr, headerBytes, salt))
	}()
	return hdr, pr, nil
}

func decryptBody(dst io.Writer, src io.Reader, master []byte, hdr Header, headerBytes, salt []byte) error {
	if len(master) != keySize {
		return fmt.Errorf("mediacrypt: master key must be %d bytes", keySize)
	}
	key, err := deriveKey(master, salt)
	if err != nil {
		return err
	}
	aead, err := newAEAD(key)
	if err != nil {
		return err
	}
	chunks := numChunks(hdr.OrigSize, hdr.ChunkSize)
	ctbuf := make([]byte, hdr.ChunkSize+tagSize)
	remaining := hdr.OrigSize
	for i := int64(0); i < chunks; i++ {
		ptLen := int64(hdr.ChunkSize)
		if remaining < ptLen {
			ptLen = remaining
		}
		ctLen := ptLen + tagSize
		if _, err := io.ReadFull(src, ctbuf[:ctLen]); err != nil {
			return fmt.Errorf("mediacrypt: read ciphertext: %w", err)
		}
		var aad []byte
		if i == 0 {
			aad = headerBytes
		}
		pt, err := aead.Open(nil, makeNonce(uint64(i), i == chunks-1), ctbuf[:ctLen], aad)
		if err != nil {
			return ErrCorrupt
		}
		if _, err := dst.Write(pt); err != nil {
			return err
		}
		remaining -= ptLen
	}
	return nil
}

func buildHeader(h Header, salt []byte) []byte {
	name := []byte(h.Name)
	out := make([]byte, 0, len(Magic)+1+1+4+8+1+len(salt)+2+len(name))
	out = append(out, Magic...)
	out = append(out, byte(version), h.KeyID)
	out = binary.BigEndian.AppendUint32(out, uint32(h.ChunkSize))
	out = binary.BigEndian.AppendUint64(out, uint64(h.OrigSize))
	out = append(out, byte(len(salt)))
	out = append(out, salt...)
	out = binary.BigEndian.AppendUint16(out, uint16(len(name)))
	out = append(out, name...)
	return out
}

// ParseHeader reads and validates the stream header from r, returning the parsed
// header, its exact bytes (for AAD), and the salt.
func ParseHeader(r io.Reader) (Header, []byte, []byte, error) {
	// Fixed prefix: magic(8) ver(1) keyID(1) chunkSize(4) origSize(8) saltLen(1)
	const fixed = len(Magic) + 1 + 1 + 4 + 8 + 1
	prefix := make([]byte, fixed)
	if _, err := io.ReadFull(r, prefix); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return Header{}, nil, nil, ErrNotEncrypted
		}
		return Header{}, nil, nil, err
	}
	if !HasMagic(prefix) {
		return Header{}, nil, nil, ErrNotEncrypted
	}
	off := len(Magic)
	ver := int(prefix[off])
	off++
	keyID := prefix[off]
	off++
	chunkSize := int(binary.BigEndian.Uint32(prefix[off : off+4]))
	off += 4
	origSize := int64(binary.BigEndian.Uint64(prefix[off : off+8]))
	off += 8
	saltLen := int(prefix[off])

	if ver != version {
		return Header{}, nil, nil, fmt.Errorf("mediacrypt: unsupported version %d", ver)
	}
	if chunkSize <= 0 || chunkSize > maxChunk {
		return Header{}, nil, nil, ErrCorrupt
	}
	if origSize < 0 {
		return Header{}, nil, nil, ErrCorrupt
	}
	if saltLen != saltSize {
		return Header{}, nil, nil, ErrCorrupt
	}

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(r, salt); err != nil {
		return Header{}, nil, nil, ErrCorrupt
	}
	var nameLenBuf [2]byte
	if _, err := io.ReadFull(r, nameLenBuf[:]); err != nil {
		return Header{}, nil, nil, ErrCorrupt
	}
	nameLen := int(binary.BigEndian.Uint16(nameLenBuf[:]))
	name := make([]byte, nameLen)
	if _, err := io.ReadFull(r, name); err != nil {
		return Header{}, nil, nil, ErrCorrupt
	}

	headerBytes := make([]byte, 0, len(prefix)+len(salt)+2+len(name))
	headerBytes = append(headerBytes, prefix...)
	headerBytes = append(headerBytes, salt...)
	headerBytes = append(headerBytes, nameLenBuf[:]...)
	headerBytes = append(headerBytes, name...)

	return Header{
		Version:   ver,
		KeyID:     keyID,
		ChunkSize: chunkSize,
		OrigSize:  origSize,
		Name:      string(name),
	}, headerBytes, salt, nil
}
