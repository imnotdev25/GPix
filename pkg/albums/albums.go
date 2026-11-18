// Package albums stores gpix's own albums and tags, per linked Google account,
// in a local SQLite database. They live only in gpix: they are not synced with
// albums in Google Photos.
package albums

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned for an album that doesn't exist (in this account).
var ErrNotFound = errors.New("albums: not found")

// Album is a named set of items.
type Album struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

// Store is a SQLite-backed album and tag store.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS albums (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL,
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS album_items (
    album_id  INTEGER NOT NULL,
    media_key TEXT NOT NULL,
    added_at  INTEGER NOT NULL,
    PRIMARY KEY (album_id, media_key)
);
CREATE TABLE IF NOT EXISTS tags (
    account_id INTEGER NOT NULL,
    media_key  TEXT NOT NULL,
    tag        TEXT NOT NULL,
    PRIMARY KEY (account_id, media_key, tag)
);
CREATE INDEX IF NOT EXISTS tags_by_tag ON tags (account_id, tag);`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// NormalizeTag trims a tag, drops a leading "#", collapses spaces and
// lowercases it, so "#Beach " and "beach" are the same tag.
func NormalizeTag(t string) string {
	t = strings.ToLower(strings.Join(strings.Fields(strings.TrimLeft(strings.TrimSpace(t), "#")), " "))
	if r := []rune(t); len(r) > 40 {
		t = string(r[:40])
	}
	return t
}

// ---- albums --------------------------------------------------------------------

// List returns an account's albums, by name.
func (s *Store) List(ctx context.Context, account int64) ([]Album, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, created_at FROM albums WHERE account_id = ? ORDER BY name COLLATE NOCASE`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Album
	for rows.Next() {
		var a Album
		var ts int64
		if err := rows.Scan(&a.ID, &a.Name, &ts); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(ts, 0)
		out = append(out, a)
	}
	return out, rows.Err()
}

// Get returns one album.
func (s *Store) Get(ctx context.Context, account, id int64) (Album, error) {
	var a Album
	var ts int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM albums WHERE id = ? AND account_id = ?`, id, account).Scan(&a.ID, &a.Name, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return Album{}, ErrNotFound
	}
	a.CreatedAt = time.Unix(ts, 0)
	return a, err
}

// Create makes a new, empty album.
func (s *Store) Create(ctx context.Context, account int64, name string) (Album, error) {
	now := time.Now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO albums (account_id, name, created_at) VALUES (?, ?, ?)`, account, name, now.Unix())
	if err != nil {
		return Album{}, err
	}
	id, _ := res.LastInsertId()
	return Album{ID: id, Name: name, CreatedAt: time.Unix(now.Unix(), 0)}, nil
}

// Rename changes an album's name.
func (s *Store) Rename(ctx context.Context, account, id int64, name string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE albums SET name = ? WHERE id = ? AND account_id = ?`, name, id, account)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes an album. The photos in it are not touched.
func (s *Store) Delete(ctx context.Context, account, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM albums WHERE id = ? AND account_id = ?`, id, account)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM album_items WHERE album_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) checkAlbum(ctx context.Context, tx *sql.Tx, account, id int64) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM albums WHERE id = ? AND account_id = ?`, id, account).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AddItems puts items into an album (already-present ones are ignored).
func (s *Store) AddItems(ctx context.Context, account, id int64, keys []string) error {
	return s.editItems(ctx, account, id, keys,
		`INSERT OR IGNORE INTO album_items (album_id, media_key, added_at) VALUES (?, ?, ?)`)
}

// RemoveItems takes items out of an album.
func (s *Store) RemoveItems(ctx context.Context, account, id int64, keys []string) error {
	return s.editItems(ctx, account, id, keys, `DELETE FROM album_items WHERE album_id = ? AND media_key = ? AND ? > 0`)
}

func (s *Store) editItems(ctx context.Context, account, id int64, keys []string, query string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.checkAlbum(ctx, tx, account, id); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().Unix()
	for _, k := range keys {
		if _, err := stmt.ExecContext(ctx, id, k, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Keys returns the media keys in an album.
func (s *Store) Keys(ctx context.Context, account, id int64) (map[string]bool, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := s.checkAlbum(ctx, tx, account, id); err != nil {
		return nil, err
	}
	return collectKeys(tx.QueryContext(ctx, `SELECT media_key FROM album_items WHERE album_id = ?`, id))
}

// Membership returns every album's media keys for an account.
func (s *Store) Membership(ctx context.Context, account int64) (map[int64][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.album_id, i.media_key FROM album_items i
		JOIN albums a ON a.id = i.album_id WHERE a.account_id = ?`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]string{}
	for rows.Next() {
		var id int64
		var k string
		if err := rows.Scan(&id, &k); err != nil {
			return nil, err
		}
		out[id] = append(out[id], k)
	}
	return out, rows.Err()
}

// ---- tags ------------------------------------------------------------------------

// AddTags tags items. Tags should already be normalized.
func (s *Store) AddTags(ctx context.Context, account int64, keys, tags []string) error {
	return s.editTags(ctx, account, keys, tags, `INSERT OR IGNORE INTO tags (account_id, media_key, tag) VALUES (?, ?, ?)`)
}

// RemoveTags untags items.
func (s *Store) RemoveTags(ctx context.Context, account int64, keys, tags []string) error {
	return s.editTags(ctx, account, keys, tags, `DELETE FROM tags WHERE account_id = ? AND media_key = ? AND tag = ?`)
}

func (s *Store) editTags(ctx context.Context, account int64, keys, tags []string, query string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, k := range keys {
		for _, t := range tags {
			if _, err := stmt.ExecContext(ctx, account, k, t); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// TagKeys returns the media keys carrying a tag.
func (s *Store) TagKeys(ctx context.Context, account int64, tag string) (map[string]bool, error) {
	return collectKeys(s.db.QueryContext(ctx, `SELECT media_key FROM tags WHERE account_id = ? AND tag = ?`, account, tag))
}

// TagsFor returns the tags of the given items.
func (s *Store) TagsFor(ctx context.Context, account int64, keys []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(keys) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(keys)+1)
	args = append(args, account)
	for _, k := range keys {
		args = append(args, k)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT media_key, tag FROM tags WHERE account_id = ? AND media_key IN (?`+
		strings.Repeat(",?", len(keys)-1)+`) ORDER BY tag`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, t string
		if err := rows.Scan(&k, &t); err != nil {
			return nil, err
		}
		out[k] = append(out[k], t)
	}
	return out, rows.Err()
}

// TagMembership returns every tag's media keys for an account.
func (s *Store) TagMembership(ctx context.Context, account int64) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tag, media_key FROM tags WHERE account_id = ?`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var t, k string
		if err := rows.Scan(&t, &k); err != nil {
			return nil, err
		}
		out[t] = append(out[t], k)
	}
	return out, rows.Err()
}

func collectKeys(rows *sql.Rows, err error) (map[string]bool, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}
