package mediacrypt

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Manager owns the gpix-managed master key and the "encrypt new uploads" toggle.
// The key lives in keyPath (0600), the toggle in statePath. Losing the key file
// means losing access to everything encrypted with it, so it should be backed up.
type Manager struct {
	mu        sync.RWMutex
	master    []byte   // active key: encrypts new uploads, first tried on decrypt
	masters   [][]byte // master + every uploaded encryption_*.key, tried on decrypt
	enabled   bool
	keyPath   string
	statePath string
}

type state struct {
	Enabled bool `json:"enabled"`
}

// Load reads (or creates) the master key at keyPath and the toggle at statePath.
func Load(keyPath, statePath string) (*Manager, error) {
	m := &Manager{keyPath: keyPath, statePath: statePath}

	key, err := os.ReadFile(keyPath)
	switch {
	case err == nil:
		if len(key) != keySize {
			return nil, errors.New("mediacrypt: key file has wrong length (expected 32 bytes)")
		}
	case errors.Is(err, os.ErrNotExist):
		key = make([]byte, keySize)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, key, 0o600); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	m.master = key
	m.masters = [][]byte{key}
	m.loadExtraKeys() // encryption_*.key files in the same directory

	if b, err := os.ReadFile(statePath); err == nil {
		var st state
		if json.Unmarshal(b, &st) == nil {
			m.enabled = st.Enabled
		}
	}
	return m, nil
}

// loadExtraKeys scans the key directory for encryption_*.key files (additional
// decryption keys, e.g. uploaded from another gpix instance) and adds any valid
// 32-byte keys as decrypt candidates.
func (m *Manager) loadExtraKeys() {
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(m.keyPath), "encryption_*.key"))
	for _, p := range matches {
		kb, err := os.ReadFile(p)
		if err != nil || len(kb) != keySize {
			continue
		}
		if !containsKey(m.masters, kb) {
			m.masters = append(m.masters, kb)
		}
	}
}

func containsKey(list [][]byte, k []byte) bool {
	for _, e := range list {
		if bytes.Equal(e, k) {
			return true
		}
	}
	return false
}

// Enabled reports whether new uploads should be encrypted.
func (m *Manager) Enabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// SetEnabled toggles encryption of new uploads and persists the choice.
func (m *Manager) SetEnabled(on bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = on
	b, _ := json.MarshalIndent(state{Enabled: on}, "", "  ")
	tmp := m.statePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.statePath)
}

// Fingerprint returns a short, non-secret identifier for the current key so the
// user can confirm which key is in use (e.g. after restoring a backup).
func (m *Manager) Fingerprint() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sum := sha256.Sum256(m.master)
	return hex.EncodeToString(sum[:8])
}

// BackupBytes returns a copy of the raw 32-byte master key for the user to store
// safely. Treat it like a password.
func (m *Manager) BackupBytes() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]byte, len(m.master))
	copy(out, m.master)
	return out
}

// Encrypt encrypts origSize bytes from src to dst using the master key.
func (m *Manager) Encrypt(dst io.Writer, src io.Reader, origSize int64, name string) error {
	m.mu.RLock()
	master := m.master
	m.mu.RUnlock()
	return Encrypt(dst, src, master, origSize, name)
}

// Decrypt streams the plaintext for an encrypted src to dst, trying every loaded
// key.
func (m *Manager) Decrypt(dst io.Writer, src io.Reader) (Header, error) {
	m.mu.RLock()
	masters := m.masters
	m.mu.RUnlock()
	return DecryptMulti(dst, src, masters)
}

// DecryptingReader returns the header and a streaming plaintext reader, trying
// every loaded key.
func (m *Manager) DecryptingReader(src io.Reader) (Header, io.ReadCloser, error) {
	m.mu.RLock()
	masters := m.masters
	m.mu.RUnlock()
	return DecryptingReaderMulti(src, masters)
}

// KeyCount returns how many keys are loaded (1 = just the active key).
func (m *Manager) KeyCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.masters)
}

// KeyInfo is a non-secret descriptor of a loaded key.
type KeyInfo struct {
	Fingerprint string
	Active      bool
}

// Keys lists loaded keys by fingerprint; the first is the active (encrypting) key.
func (m *Manager) Keys() []KeyInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]KeyInfo, 0, len(m.masters))
	for i, k := range m.masters {
		sum := sha256.Sum256(k)
		out = append(out, KeyInfo{Fingerprint: hex.EncodeToString(sum[:8]), Active: i == 0})
	}
	return out
}

// AddKeyRaw stores an additional decryption key — raw 32 bytes, or hex/base64 of
// 32 bytes — as encryption_<random>.key and immediately uses it for decryption.
// Returns the created filename, or "" if the key was already loaded.
func (m *Manager) AddKeyRaw(raw []byte) (string, error) {
	key, err := normalizeKey(raw)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if containsKey(m.masters, key) {
		return "", nil
	}
	name := "encryption_" + randSuffix(7) + ".key"
	if err := os.WriteFile(filepath.Join(filepath.Dir(m.keyPath), name), key, 0o600); err != nil {
		return "", err
	}
	m.masters = append(m.masters, key)
	return name, nil
}

func normalizeKey(raw []byte) ([]byte, error) {
	if len(raw) == keySize {
		out := make([]byte, keySize)
		copy(out, raw)
		return out, nil
	}
	s := strings.TrimSpace(string(raw))
	if len(s) == 2*keySize {
		if b, err := hex.DecodeString(s); err == nil && len(b) == keySize {
			return b, nil
		}
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) == keySize {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil && len(b) == keySize {
		return b, nil
	}
	return nil, errors.New("mediacrypt: key must be 32 raw bytes, or hex/base64 of 32 bytes")
}

func randSuffix(n int) string {
	const alpha = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alpha[int(b[i])%len(alpha)]
	}
	return string(b)
}
