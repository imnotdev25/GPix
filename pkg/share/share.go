// Package share persists password-protected, expiring share links in SQLite.
// A share points at a single Google Photos media key; the public endpoints in
// pkg/web resolve, decrypt (if needed) and serve it without exposing the user's
// session or encryption key.
//
// SQLite is provided by the pure-Go modernc.org/sqlite driver, so the static
// (CGO-free) build keeps working. Run `go get modernc.org/sqlite` once.
package share

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a token does not exist.
var ErrNotFound = errors.New("share: not found")

// Share is a single share link.
type Share struct {
	Token         string
	MediaKey      string
	FileName      string
	IsVideo       bool
	HasPassword   bool
	ExpiresAt     time.Time // zero = never
	MaxDownloads  int64     // 0 = unlimited
	Downloads     int64
	AllowOriginal bool
	CreatedAt     time.Time

	passwordHash string // bcrypt hash, kept internal to the package
}

// CreateParams describes a new share.
type CreateParams struct {
	MediaKey      string
	FileName      string
	IsVideo       bool
	Password      string        // empty = no password
	TTL           time.Duration // 0 = never expires
	MaxDownloads  int64         // 0 = unlimited
	AllowOriginal bool
}

// Store is a SQLite-backed share store, safe for concurrent use (database/sql
// pooling plus SQLite's own locking).
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and ensures the
// schema exists.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: serialise writers; WAL allows concurrent reads
	s := &Store{db: db}
	if err := s.init(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) init(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS shares (
    token          TEXT PRIMARY KEY,
    media_key      TEXT NOT NULL,
    file_name      TEXT NOT NULL,
    is_video       INTEGER NOT NULL DEFAULT 0,
    password_hash  TEXT,
    expires_at     INTEGER,
    max_downloads  INTEGER NOT NULL DEFAULT 0,
    downloads      INTEGER NOT NULL DEFAULT 0,
    allow_original INTEGER NOT NULL DEFAULT 1,
    created_at     INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_shares_created ON shares(created_at);`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Create inserts a new share and returns it.
func (s *Store) Create(ctx context.Context, p CreateParams) (Share, error) {
	token, err := newToken()
	if err != nil {
		return Share{}, err
	}
	var pwHash sql.NullString
	if p.Password != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(p.Password), 12)
		if err != nil {
			return Share{}, err
		}
		pwHash = sql.NullString{String: string(h), Valid: true}
	}
	var expires sql.NullInt64
	if p.TTL > 0 {
		expires = sql.NullInt64{Int64: time.Now().Add(p.TTL).Unix(), Valid: true}
	}
	now := time.Now()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO shares (token, media_key, file_name, is_video, password_hash, expires_at, max_downloads, downloads, allow_original, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		token, p.MediaKey, p.FileName, b2i(p.IsVideo), pwHash, expires, p.MaxDownloads, b2i(p.AllowOriginal), now.Unix())
	if err != nil {
		return Share{}, err
	}
	return s.Get(ctx, token)
}

// Get returns a single share by token.
func (s *Store) Get(ctx context.Context, token string) (Share, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT token, media_key, file_name, is_video, password_hash, expires_at, max_downloads, downloads, allow_original, created_at
FROM shares WHERE token = ?`, token)
	return scanShare(row)
}

// List returns all shares, newest first.
func (s *Store) List(ctx context.Context) ([]Share, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT token, media_key, file_name, is_video, password_hash, expires_at, max_downloads, downloads, allow_original, created_at
FROM shares ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Share
	for rows.Next() {
		sh, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// Delete removes a share.
func (s *Store) Delete(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM shares WHERE token = ?`, token)
	return err
}

// RecordDownload atomically increments the download counter.
func (s *Store) RecordDownload(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE shares SET downloads = downloads + 1 WHERE token = ?`, token)
	return err
}

// VerifyPassword reports whether pw matches the share's password. A share with
// no password always returns true.
func (sh Share) VerifyPassword(pw string) bool {
	if !sh.HasPassword {
		return true
	}
	return bcrypt.CompareHashAndPassword([]byte(sh.passwordHash), []byte(pw)) == nil
}

// Active reports whether the share can still be served at time now.
func (sh Share) Active(now time.Time) bool {
	if !sh.ExpiresAt.IsZero() && now.After(sh.ExpiresAt) {
		return false
	}
	if sh.MaxDownloads > 0 && sh.Downloads >= sh.MaxDownloads {
		return false
	}
	return true
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanShare(sc rowScanner) (Share, error) {
	var (
		sh       Share
		isVideo  int64
		pwHash   sql.NullString
		expires  sql.NullInt64
		allowOrg int64
		created  int64
	)
	err := sc.Scan(&sh.Token, &sh.MediaKey, &sh.FileName, &isVideo, &pwHash, &expires, &sh.MaxDownloads, &sh.Downloads, &allowOrg, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Share{}, ErrNotFound
	}
	if err != nil {
		return Share{}, err
	}
	sh.IsVideo = isVideo != 0
	sh.AllowOriginal = allowOrg != 0
	sh.HasPassword = pwHash.Valid && pwHash.String != ""
	sh.passwordHash = pwHash.String
	if expires.Valid {
		sh.ExpiresAt = time.Unix(expires.Int64, 0)
	}
	sh.CreatedAt = time.Unix(created, 0)
	return sh, nil
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
