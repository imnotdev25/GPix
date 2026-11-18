// Package users stores gpix sign-in accounts: email + password accounts made
// through sign-up, and single sign-on (OIDC) identities. Together with the
// email allowlist and account cap it decides who may get into gpix.
package users

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var (
	// ErrNotFound is returned when no such account exists.
	ErrNotFound = errors.New("users: not found")
	// ErrExists is returned when signing up with an email that already has an account.
	ErrExists = errors.New("users: account already exists")
	// ErrClosed is returned when the account cap is reached.
	ErrClosed = errors.New("users: sign-up is closed")
	// ErrBadPassword is returned for a wrong email or password.
	ErrBadPassword = errors.New("users: wrong email or password")
)

const bcryptCost = 12

// User is a sign-in account.
type User struct {
	Subject      string // "local:<email>" for password accounts, else the OIDC subject
	Email        string // for imported legacy logins, the old username
	Name         string
	PasswordHash string // bcrypt; empty for single sign-on accounts
	CreatedAt    time.Time
}

// Store is a SQLite-backed account store.
type Store struct {
	db *sql.DB
}

// Open opens (creating or upgrading if needed) the database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // also serialises Register's count-then-insert
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS users (
    subject    TEXT PRIMARY KEY,
    email      TEXT,
    name       TEXT,
    created_at INTEGER NOT NULL
);`); err != nil {
		return err
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'password_hash'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := db.Exec(`ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	_, err := db.Exec(`CREATE INDEX IF NOT EXISTS users_email ON users (lower(email))`)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

const userCols = `subject, COALESCE(email, ''), COALESCE(name, ''), password_hash, created_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var (
		u  User
		ts int64
	)
	if err := row.Scan(&u.Subject, &u.Email, &u.Name, &u.PasswordHash, &ts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	u.CreatedAt = time.Unix(ts, 0)
	return u, nil
}

// Get returns an account by subject.
func (s *Store) Get(ctx context.Context, subject string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE subject = ?`, subject))
}

// GetByEmail finds an account by email (or imported login name), ignoring
// case. A password account wins over a single sign-on one with the same email.
func (s *Store) GetByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM users WHERE lower(email) = lower(?)
		 ORDER BY password_hash = '', created_at LIMIT 1`, strings.TrimSpace(email)))
}

// Count returns the number of accounts.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// Register creates an account unless the cap is reached (maxUsers 0 = no cap).
// Counting and inserting happen in one transaction, so two sign-ups at the same
// moment can't both take the last place. It returns ErrExists for a subject
// that's already registered, or a password account whose email is taken.
func (s *Store) Register(ctx context.Context, u User, maxUsers int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE subject = ?`, u.Subject).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrExists
	}
	if u.PasswordHash != "" {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE lower(email) = lower(?)`, u.Email).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrExists
		}
	}
	if maxUsers > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
			return err
		}
		if n >= maxUsers {
			return ErrClosed
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (subject, email, name, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.Subject, u.Email, u.Name, u.PasswordHash, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// dummyHash makes a failed lookup cost as much as a wrong password, so the
// response time doesn't reveal whether an account exists.
var dummyHash = sync.OnceValue(func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("gpix timing equaliser"), bcryptCost)
	return h
})

// Authenticate checks an email (or imported login name) and password.
func (s *Store) Authenticate(ctx context.Context, email, password string) (User, error) {
	u, err := s.GetByEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return User{}, err
	}
	if err != nil || u.PasswordHash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		return User{}, ErrBadPassword
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return User{}, ErrBadPassword
	}
	return u, nil
}

// ImportLogin makes sure a password account exists for login (an email or a
// plain name) with the given bcrypt hash, creating it or updating its password.
// It carries over the old GPIX_USERNAME / GPIX_PASSWORD_HASH settings, so it
// ignores the account cap. It reports whether it created the account.
func (s *Store) ImportLogin(ctx context.Context, login, hash string) (bool, error) {
	login = strings.TrimSpace(login)
	subject := "local:" + strings.ToLower(login)
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE subject = ?`, hash, subject)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return false, nil
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO users (subject, email, name, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		subject, login, login, hash, time.Now().Unix())
	return err == nil, err
}

// List returns all accounts, newest first.
func (s *Store) List(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// HashPassword returns a bcrypt hash for a new password (at most 72 bytes).
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(h), err
}

// Allowlisted reports whether email is permitted by the allowlist. An empty
// list allows everyone. Entries may be exact emails ("a@b.com") or domains
// ("@b.com" or "b.com"); matching is case-insensitive.
func Allowlisted(email string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	domain := ""
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		domain = email[i+1:]
	}
	for _, entry := range allowlist {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "@") {
			if domain == entry[1:] {
				return true
			}
			continue
		}
		if !strings.Contains(entry, "@") { // bare domain
			if domain == entry {
				return true
			}
			continue
		}
		if email == entry {
			return true
		}
	}
	return false
}
