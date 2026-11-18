// Package uploads keeps a local record of the items gpix itself committed to
// Google Photos, per linked account.
//
// Google Photos' listing carries no "uploaded by" field, and gpix presents
// itself as a Pixel phone, so a plain photo uploaded by gpix is
// indistinguishable from one uploaded by the user's phone. This ledger is how
// gpix tells them apart later (e.g. for "delete everything gpix uploaded").
// It only knows about uploads made after it was introduced; Since reports when
// that was.
package uploads

import (
	"context"
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// Store is a SQLite-backed upload ledger. It is safe for concurrent use.
type Store struct {
	db    *sql.DB
	since time.Time
}

// Open opens (creating if needed) the ledger database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS uploads (
    account_id  INTEGER NOT NULL,
    media_key   TEXT    NOT NULL,
    filename    TEXT    NOT NULL,
    uploaded_at INTEGER NOT NULL,
    PRIMARY KEY (account_id, media_key)
);
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v INTEGER NOT NULL);
INSERT OR IGNORE INTO meta (k, v) VALUES ('created_at', CAST(strftime('%s','now') AS INTEGER));`); err != nil {
		db.Close()
		return nil, err
	}
	var created int64
	if err := db.QueryRow(`SELECT v FROM meta WHERE k = 'created_at'`).Scan(&created); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, since: time.Unix(created, 0)}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Since is when the ledger started recording. Items gpix uploaded before this
// are not in it.
func (s *Store) Since() time.Time { return s.since }

// Record notes that gpix uploaded mediaKey to the given account.
func (s *Store) Record(ctx context.Context, accountID int64, mediaKey, filename string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO uploads (account_id, media_key, filename, uploaded_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (account_id, media_key) DO UPDATE SET filename = excluded.filename`,
		accountID, mediaKey, filename, time.Now().Unix())
	return err
}

// Keys returns the media keys recorded for an account.
func (s *Store) Keys(ctx context.Context, accountID int64) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT media_key FROM uploads WHERE account_id = ?`, accountID)
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

// Forget removes the given media keys from an account's record (e.g. after
// they were deleted).
func (s *Store) Forget(ctx context.Context, accountID int64, mediaKeys []string) error {
	if len(mediaKeys) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM uploads WHERE account_id = ? AND media_key = ?`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, k := range mediaKeys {
		if _, err := stmt.ExecContext(ctx, accountID, k); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
