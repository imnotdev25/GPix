// Package gwcreds is a tiny, thread-safe, file-backed store for the gateway
// credentials managed from the web UI:
//
//   - S3 access key id + secret access key (used to verify SigV4),
//   - a WebDAV "app password" (an alternative to the main login password).
//
// Credentials are persisted as JSON next to the web secret.key, with 0600
// permissions — the same trust model the project already uses for GP_AUTH_DATA
// and secret.key (single user, local file). The S3 secret in particular MUST be
// recoverable in plaintext because SigV4 verification recomputes the signature
// from it, so it is stored as-is rather than hashed.
package gwcreds

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"sync"
)

// Data is the on-disk representation.
type Data struct {
	S3AccessKey    string `json:"s3_access_key"`
	S3SecretKey    string `json:"s3_secret_key"`
	WebDAVPassword string `json:"webdav_password"`
}

// Store is a concurrency-safe, persistent credential holder.
type Store struct {
	mu   sync.RWMutex
	path string
	data Data
}

// Load reads the store from path, creating an empty in-memory store if the file
// does not exist yet.
func Load(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if uerr := json.Unmarshal(b, &s.data); uerr != nil {
			return nil, uerr
		}
	case errors.Is(err, os.ErrNotExist):
		// fresh store
	default:
		return nil, err
	}
	return s, nil
}

// Snapshot returns a copy of the current data.
func (s *Store) Snapshot() Data {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// Seed fills any empty fields from the provided defaults (e.g. values from the
// config file on first run) and persists if anything changed. Existing,
// non-empty values are never overwritten.
func (s *Store) Seed(access, secret, davPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	if s.data.S3AccessKey == "" && access != "" {
		s.data.S3AccessKey = access
		changed = true
	}
	if s.data.S3SecretKey == "" && secret != "" {
		s.data.S3SecretKey = secret
		changed = true
	}
	if s.data.WebDAVPassword == "" && davPassword != "" {
		s.data.WebDAVPassword = davPassword
		changed = true
	}
	if !changed {
		return nil
	}
	return s.save()
}

// Lookup implements the S3 credential-provider contract: given an access key id
// it returns the matching secret. The comparison is constant-time.
func (s *Store) Lookup(accessKey string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.S3AccessKey == "" || accessKey == "" {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(accessKey), []byte(s.data.S3AccessKey)) != 1 {
		return "", false
	}
	return s.data.S3SecretKey, true
}

// S3 returns the current access key id and secret.
func (s *Store) S3() (access, secret string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.S3AccessKey, s.data.S3SecretKey
}

// WebDAVPassword returns the current WebDAV app password (may be empty).
func (s *Store) WebDAVPassword() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.WebDAVPassword
}

// CheckWebDAVPassword reports whether the supplied password matches the stored
// app password (constant-time). Returns false when no app password is set.
func (s *Store) CheckWebDAVPassword(pass string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.WebDAVPassword == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(pass), []byte(s.data.WebDAVPassword)) == 1
}

// RegenerateS3 creates a fresh access key id + secret, persists them and returns
// the new pair.
func (s *Store) RegenerateS3() (access, secret string, err error) {
	access = "GPIX" + randBase32(16) // 20 chars total, like an AWS access key id
	secret = randBase64Std(30)       // 40 chars, like an AWS secret access key
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.S3AccessKey = access
	s.data.S3SecretKey = secret
	if err := s.save(); err != nil {
		return "", "", err
	}
	return access, secret, nil
}

// RegenerateWebDAV creates a fresh WebDAV app password, persists it and returns
// it.
func (s *Store) RegenerateWebDAV() (string, error) {
	pass := randBase64URL(18) // 24 url-safe chars, safe for HTTP Basic
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.WebDAVPassword = pass
	if err := s.save(); err != nil {
		return "", err
	}
	return pass, nil
}

// ClearS3 removes the S3 credentials.
func (s *Store) ClearS3() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.S3AccessKey = ""
	s.data.S3SecretKey = ""
	return s.save()
}

// ClearWebDAV removes the WebDAV app password.
func (s *Store) ClearWebDAV() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.WebDAVPassword = ""
	return s.save()
}

// save writes the store atomically. The caller must hold s.mu.
func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// --- random helpers ---

const base32Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

func randBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("gwcreds: crypto/rand failed: " + err.Error())
	}
	return b
}

// randBase32 returns n uppercase base32 characters.
func randBase32(n int) string {
	raw := randBytes(n)
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = base32Alphabet[int(raw[i])%len(base32Alphabet)]
	}
	return string(out)
}

// randBase64Std returns the standard-base64 encoding of n random bytes.
func randBase64Std(n int) string {
	return base64.StdEncoding.EncodeToString(randBytes(n))
}

// randBase64URL returns the url-safe, unpadded base64 encoding of n random bytes.
func randBase64URL(n int) string {
	return base64.RawURLEncoding.EncodeToString(randBytes(n))
}
