package mediacrypt

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
)

// Manager owns the gpix-managed master key and the "encrypt new uploads" toggle.
// The key lives in keyPath (0600), the toggle in statePath. Losing the key file
// means losing access to everything encrypted with it, so it should be backed up.
type Manager struct {
	mu        sync.RWMutex
	master    []byte
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

	if b, err := os.ReadFile(statePath); err == nil {
		var st state
		if json.Unmarshal(b, &st) == nil {
			m.enabled = st.Enabled
		}
	}
	return m, nil
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

// Decrypt streams the plaintext for an encrypted src to dst.
func (m *Manager) Decrypt(dst io.Writer, src io.Reader) (Header, error) {
	m.mu.RLock()
	master := m.master
	m.mu.RUnlock()
	return Decrypt(dst, src, master)
}

// DecryptingReader returns the header and a streaming plaintext reader.
func (m *Manager) DecryptingReader(src io.Reader) (Header, io.ReadCloser, error) {
	m.mu.RLock()
	master := m.master
	m.mu.RUnlock()
	return DecryptingReader(src, master)
}
