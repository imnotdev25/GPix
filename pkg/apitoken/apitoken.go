// Package apitoken stores long-lived API tokens used by native clients (e.g.
// the gpix mobile/desktop app) to authenticate against the /api/v1 JSON API.
//
// Only a SHA-256 hash of each token is stored; the plaintext is shown once at
// creation time and never persisted. Tokens are revocable by deleting the row.
package apitoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"time"

	_ "modernc.org/sqlite"
)

// Prefix identifies gpix API tokens at a glance.
const tokenPrefix = "gpix_"

// Token is a stored API-token record (never includes the secret).
type Token struct {
	ID        int64     `json:"id"`
	Label     string    `json:"label"`
	Preview   string    `json:"preview"` // first few chars, for display
	CreatedAt time.Time `json:"createdAt"`
	LastUsed  time.Time `json:"lastUsed"`
}

// Store is a SQLite-backed API-token store.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the token database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS api_tokens (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    label      TEXT NOT NULL,
    preview    TEXT NOT NULL,
    hash       TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    last_used  INTEGER NOT NULL DEFAULT 0
);`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// Create mints a new token. The plaintext secret is returned ONCE and is not
// recoverable afterwards.
func (s *Store) Create(ctx context.Context, label string) (secret string, tok Token, err error) {
	if label == "" {
		label = "app token"
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", Token{}, err
	}
	secret = tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	preview := secret[:len(tokenPrefix)+6] + "…"
	now := time.Now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (label, preview, hash, created_at) VALUES (?, ?, ?, ?)`,
		label, preview, hashToken(secret), now.Unix())
	if err != nil {
		return "", Token{}, err
	}
	id, _ := res.LastInsertId()
	return secret, Token{ID: id, Label: label, Preview: preview, CreatedAt: now}, nil
}

// Verify checks a presented secret and, on success, returns the token record and
// updates its last-used timestamp.
func (s *Store) Verify(ctx context.Context, secret string) (Token, bool) {
	var t Token
	var created, last int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, label, preview, created_at, last_used FROM api_tokens WHERE hash = ?`,
		hashToken(secret)).Scan(&t.ID, &t.Label, &t.Preview, &created, &last)
	if err != nil {
		return Token{}, false
	}
	t.CreatedAt = time.Unix(created, 0)
	if last > 0 {
		t.LastUsed = time.Unix(last, 0)
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used = ? WHERE id = ?`, time.Now().Unix(), t.ID)
	return t, true
}

// List returns all tokens, newest first.
func (s *Store) List(ctx context.Context) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, label, preview, created_at, last_used FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Token
	for rows.Next() {
		var t Token
		var created, last int64
		if err := rows.Scan(&t.ID, &t.Label, &t.Preview, &created, &last); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(created, 0)
		if last > 0 {
			t.LastUsed = time.Unix(last, 0)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Revoke deletes a token by id.
func (s *Store) Revoke(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	return err
}
