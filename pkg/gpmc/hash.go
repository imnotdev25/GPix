package gpmc

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

func HashFile(path string) (digest []byte, b64 string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	h := sha1.New()
	buf := make([]byte, 1<<20)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return nil, "", err
	}
	sum := h.Sum(nil)
	return sum, base64.StdEncoding.EncodeToString(sum), nil
}

func ConvertSHA1(in any) (digest []byte, b64 string, err error) {
	switch v := in.(type) {
	case []byte:
		if len(v) != sha1.Size {
			return nil, "", fmt.Errorf("gpmc: sha1 bytes must be %d long, got %d", sha1.Size, len(v))
		}
		return v, base64.StdEncoding.EncodeToString(v), nil
	case string:
		if len(v) == 2*sha1.Size && isHex(v) {
			b, decErr := hex.DecodeString(v)
			if decErr != nil {
				return nil, "", decErr
			}
			return b, base64.StdEncoding.EncodeToString(b), nil
		}
		b, decErr := base64.StdEncoding.DecodeString(v)
		if decErr != nil {
			return nil, "", decErr
		}
		if len(b) != sha1.Size {
			return nil, "", fmt.Errorf("gpmc: decoded sha1 must be %d bytes, got %d", sha1.Size, len(b))
		}
		return b, v, nil
	default:
		return nil, "", errors.New("gpmc: ConvertSHA1 expects []byte or string")
	}
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
