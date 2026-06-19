package store

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"sort"
	"sync"
	"time"
)

// MemBackend is an in-memory Backend used by tests and the standalone gateway
// test harness (cmd/gpix-gateway-test). It depends only on the standard
// library, so it builds and runs without Google Photos credentials.
type MemBackend struct {
	mu      sync.RWMutex
	objects map[string]memObject
}

type memObject struct {
	data        []byte
	modTime     time.Time
	etag        string
	contentType string
}

// NewMemBackend returns an empty in-memory backend.
func NewMemBackend() *MemBackend {
	return &MemBackend{objects: make(map[string]memObject)}
}

func (m *MemBackend) Name() string { return "memory" }

func (m *MemBackend) List(_ context.Context) ([]Object, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Object, 0, len(m.objects))
	for k, o := range m.objects {
		out = append(out, Object{
			Key:         k,
			Size:        int64(len(o.data)),
			ModTime:     o.modTime,
			ETag:        o.etag,
			ContentType: o.contentType,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (m *MemBackend) Stat(_ context.Context, key string) (Object, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.objects[key]
	if !ok {
		return Object{}, ErrNotFound
	}
	return Object{
		Key:         key,
		Size:        int64(len(o.data)),
		ModTime:     o.modTime,
		ETag:        o.etag,
		ContentType: o.contentType,
	}, nil
}

func (m *MemBackend) Get(_ context.Context, key string) (io.ReadCloser, Object, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.objects[key]
	if !ok {
		return nil, Object{}, ErrNotFound
	}
	obj := Object{
		Key:         key,
		Size:        int64(len(o.data)),
		ModTime:     o.modTime,
		ETag:        o.etag,
		ContentType: o.contentType,
	}
	return io.NopCloser(bytes.NewReader(o.data)), obj, nil
}

func (m *MemBackend) Put(_ context.Context, key string, r io.Reader, _ int64, contentType string) (Object, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Object{}, err
	}
	sum := sha1.Sum(data)
	etag := hex.EncodeToString(sum[:])
	now := time.Now().UTC()
	m.mu.Lock()
	m.objects[key] = memObject{data: data, modTime: now, etag: etag, contentType: contentType}
	m.mu.Unlock()
	return Object{
		Key:         key,
		Size:        int64(len(data)),
		ModTime:     now,
		ETag:        etag,
		ContentType: contentType,
	}, nil
}

func (m *MemBackend) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objects[key]; !ok {
		return ErrNotFound
	}
	delete(m.objects, key)
	return nil
}
