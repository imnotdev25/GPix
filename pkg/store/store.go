// Package store defines a minimal object-storage abstraction that the S3 and
// WebDAV gateways are built on. The production implementation lives in
// pkg/store/gpmcstore (backed by Google Photos); pkg/store also ships an
// in-memory backend (MemBackend) used by tests and the standalone gateway
// test harness.
//
// The model is deliberately flat: a single implicit bucket holding objects
// keyed by name. Google Photos has no real folder hierarchy, so neither does
// this abstraction. The S3 layer synthesises CommonPrefixes from a delimiter,
// and the WebDAV layer presents every object inside one root collection.
package store

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound is returned by Stat/Get/Delete when the key does not exist.
var ErrNotFound = errors.New("store: object not found")

// Object is the metadata describing a single stored object.
type Object struct {
	Key         string    // object key (for the gpmc backend, the original filename)
	Size        int64     // size in bytes of the logical object (post-unwrap for disguised files)
	ModTime     time.Time // last modification time
	ETag        string    // opaque entity tag WITHOUT surrounding quotes (gpmc: hex SHA-1)
	ContentType string    // MIME type; may be empty if unknown
}

// Backend is the storage contract the gateways depend on. Implementations must
// be safe for concurrent use.
type Backend interface {
	// Name is a short human-readable identifier for logging.
	Name() string

	// List returns every object in the store (flat namespace). Callers that
	// need prefix/delimiter semantics filter the result themselves.
	List(ctx context.Context) ([]Object, error)

	// Stat returns metadata for a single key, or ErrNotFound.
	Stat(ctx context.Context, key string) (Object, error)

	// Get opens the object for reading. The returned ReadCloser must be closed
	// by the caller. The returned Object carries the best-known metadata
	// (size/content-type) for the stream.
	Get(ctx context.Context, key string) (io.ReadCloser, Object, error)

	// Put stores the bytes read from r under key. size is the caller's best
	// estimate of the content length, or -1 when unknown (e.g. chunked
	// transfer); implementations must not rely on it being accurate.
	// contentType is advisory and may be empty.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (Object, error)

	// Delete removes the object. Removing a missing key returns ErrNotFound.
	Delete(ctx context.Context, key string) error
}
