package users

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestRegisterCapAndAuthenticate(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	h, _ := bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost)
	owner := User{Subject: "local:me@example.com", Email: "me@example.com", PasswordHash: string(h)}
	if err := s.Register(ctx, owner, 1); err != nil {
		t.Fatal(err)
	}
	// Same email again: exists, even with different case.
	dup := owner
	dup.Subject, dup.Email = "local:ME@example.com", "ME@example.com"
	if err := s.Register(ctx, dup, 0); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate email: err = %v", err)
	}
	// Cap of 1 reached: closed for anyone else, including single sign-on.
	if err := s.Register(ctx, User{Subject: "oidc|1", Email: "x@example.com"}, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("over cap: err = %v", err)
	}
	// No cap: allowed.
	if err := s.Register(ctx, User{Subject: "oidc|1", Email: "x@example.com"}, 0); err != nil {
		t.Fatalf("no cap: %v", err)
	}

	if _, err := s.Authenticate(ctx, "Me@Example.com", "correct horse"); err != nil {
		t.Fatalf("good password: %v", err)
	}
	if _, err := s.Authenticate(ctx, "me@example.com", "wrong"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("bad password: err = %v", err)
	}
	if _, err := s.Authenticate(ctx, "nobody@example.com", "x"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("unknown user: err = %v", err)
	}
	if _, err := s.Authenticate(ctx, "x@example.com", ""); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("SSO-only account must not sign in with a password: err = %v", err)
	}
}

func TestImportLoginAndMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "users.db")

	// An old database without the password_hash column.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (subject TEXT PRIMARY KEY, email TEXT, name TEXT, created_at INTEGER NOT NULL);
		INSERT INTO users VALUES ('oidc|old', 'old@example.com', 'Old', 1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if u, err := s.Get(ctx, "oidc|old"); err != nil || u.Email != "old@example.com" {
		t.Fatalf("migrated user = %+v, %v", u, err)
	}

	h1, _ := bcrypt.GenerateFromPassword([]byte("first"), bcrypt.MinCost)
	created, err := s.ImportLogin(ctx, "admin", string(h1))
	if err != nil || !created {
		t.Fatalf("import: created=%v err=%v", created, err)
	}
	if _, err := s.Authenticate(ctx, "admin", "first"); err != nil {
		t.Fatalf("imported login: %v", err)
	}
	h2, _ := bcrypt.GenerateFromPassword([]byte("second"), bcrypt.MinCost)
	if created, err := s.ImportLogin(ctx, "Admin", string(h2)); err != nil || created {
		t.Fatalf("re-import should update: created=%v err=%v", created, err)
	}
	if _, err := s.Authenticate(ctx, "admin", "second"); err != nil {
		t.Fatalf("updated password: %v", err)
	}
	if n, _ := s.Count(ctx); n != 2 {
		t.Fatalf("count = %d", n)
	}
}
