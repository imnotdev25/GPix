// Package accounts lets one gpix instance hold several linked Google Photos
// accounts and switch which one is "active". Every surface (web UI, S3, WebDAV,
// importer) reads through the active account, so switching swaps the whole
// library atomically.
//
// Each account's auth_data is the credential for a Google Photos account —
// treat accounts.db like a password file. It is created 0600 and git-ignored,
// matching how GP_AUTH_DATA lives in .env.
package accounts

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Account is a linked Google account (never carries auth_data outward).
type Account struct {
	ID      int64     `json:"id"`
	Label   string    `json:"label"`
	AddedAt time.Time `json:"addedAt"`
	Active  bool      `json:"active"`
}

// Store persists linked accounts.
type Store struct{ db *sql.DB }

// Open opens (creating if needed) the accounts database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS accounts (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    label     TEXT NOT NULL,
    auth_data TEXT NOT NULL,
    added_at  INTEGER NOT NULL,
    active    INTEGER NOT NULL DEFAULT 0
);`); err != nil {
		db.Close()
		return nil, err
	}
	_ = os.Chmod(path, 0o600) // holds credentials
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// List returns all linked accounts, oldest first.
func (s *Store) List(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, label, added_at, active FROM accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		var added int64
		var active int
		if err := rows.Scan(&a.ID, &a.Label, &added, &active); err != nil {
			return nil, err
		}
		a.AddedAt = time.Unix(added, 0)
		a.Active = active == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// Count returns how many accounts are linked.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&n)
	return n, err
}

// Add links a new account. The first account added becomes active.
func (s *Store) Add(ctx context.Context, label, authData string) (Account, error) {
	label = strings.TrimSpace(label)
	authData = strings.TrimSpace(authData)
	if authData == "" {
		return Account{}, errors.New("accounts: auth_data is required")
	}
	if label == "" {
		label = "Account"
	}
	n, err := s.Count(ctx)
	if err != nil {
		return Account{}, err
	}
	now := time.Now()
	active := 0
	if n == 0 {
		active = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO accounts (label, auth_data, added_at, active) VALUES (?, ?, ?, ?)`,
		label, authData, now.Unix(), active)
	if err != nil {
		return Account{}, err
	}
	id, _ := res.LastInsertId()
	return Account{ID: id, Label: label, AddedAt: now, Active: active == 1}, nil
}

// AuthData returns the stored credential for one account.
func (s *Store) AuthData(ctx context.Context, id int64) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT auth_data FROM accounts WHERE id = ?`, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("accounts: no such account")
	}
	return v, err
}

// FindByAuthData returns the id of an account with this credential, if linked.
func (s *Store) FindByAuthData(ctx context.Context, authData string) (int64, bool) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM accounts WHERE auth_data = ?`, strings.TrimSpace(authData)).Scan(&id)
	return id, err == nil
}

// SetActive makes one account active (and all others inactive).
func (s *Store) SetActive(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var one int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM accounts WHERE id = ?`, id).Scan(&one); err != nil {
		return errors.New("accounts: no such account")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET active = 0`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET active = 1 WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ActiveID returns the active account id (or the lowest id if none is marked).
func (s *Store) ActiveID(ctx context.Context) (int64, bool) {
	var id int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT id FROM accounts WHERE active = 1 LIMIT 1`).Scan(&id); err == nil {
		return id, true
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT id FROM accounts ORDER BY id LIMIT 1`).Scan(&id); err == nil {
		return id, true
	}
	return 0, false
}

// Remove unlinks an account. The active account cannot be removed.
func (s *Store) Remove(ctx context.Context, id int64) error {
	var active int
	if err := s.db.QueryRowContext(ctx, `SELECT active FROM accounts WHERE id = ?`, id).Scan(&active); err != nil {
		return errors.New("accounts: no such account")
	}
	if active == 1 {
		return errors.New("accounts: switch to another account before removing this one")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, id)
	return err
}

// Rename changes an account's label.
func (s *Store) Rename(ctx context.Context, id int64, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return errors.New("accounts: label is required")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE accounts SET label = ? WHERE id = ?`, label, id)
	return err
}
