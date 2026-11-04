package owner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestStoreRejectsUnsafeFiles(t *testing.T) {
	for _, mode := range []string{"directory", "public", "symlink", "occupied", "parent"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "v1")
			if mode == "parent" {
				if err := os.WriteFile(dir, nil, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(dir, "owner.db")
			switch mode {
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "public":
				// The fixture verifies that the store rejects public state files.
				if err := os.WriteFile(path, nil, 0644); err != nil { // #nosec G306
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(filepath.Join(root, "other"), path); err != nil {
					t.Fatal(err)
				}
			case "occupied":
				s, err := Open(root)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = s.Close() }()
			}
			if s, err := Open(root); err == nil {
				_ = s.Close()
				t.Fatal("unsafe state opens")
			}
		})
	}
}

func TestStorePreservesTheVersionOneFixtureAcrossRestart(t *testing.T) {
	directory := t.TempDir()
	s, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("testdata/identity-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(stateBucket).Put(identityKey, fixture) }); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Verify([]byte("synthetic-fixture")); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover([]byte("synthetic-replacement")); err != nil {
		t.Fatal(err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		identity, err := readIdentity(tx)
		if err != nil {
			return err
		}
		if identity.ID != "00112233445566778899aabbccddeeff" || identity.Version != 1 {
			t.Fatal("recovery changes fixture identity")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRejectsInvalidIdentityFixtures(t *testing.T) {
	s := sessionStore(t)
	var identity Identity
	if err := s.db.View(func(tx *bolt.Tx) error { var err error; identity, err = readIdentity(tx); return err }); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"json", "version", "id", "created", "algorithm", "salt", "digest"} {
		t.Run(field, func(t *testing.T) {
			fixture := identity
			switch field {
			case "version":
				fixture.Version++
			case "id":
				fixture.ID = ""
			case "created":
				fixture.CreatedAt = time.Time{}
			case "algorithm":
				fixture.Password.Algorithm = "unknown"
			case "salt":
				fixture.Password.Salt = "bad"
			case "digest":
				fixture.Password.Digest = "bad"
			}
			data, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			if field == "json" {
				data = []byte("not JSON")
			}
			if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(stateBucket).Put(identityKey, data) }); err != nil {
				t.Fatal(err)
			}
			if err := s.Verify([]byte("synthetic")); err == nil {
				t.Fatal("invalid fixture authenticates")
			}
			if err := s.Recover([]byte("replacement")); err == nil {
				t.Fatal("recovery accepts invalid fixture")
			}
		})
	}
}

func TestStoreRejectsInvalidPasswordsAndClosedState(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, password := range [][]byte{nil, []byte(strings.Repeat("x", 1025))} {
		if err := s.Initialize(password); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	if err := s.Recover([]byte("synthetic")); !errors.Is(err, ErrCredentials) {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize([]byte("synthetic")); err == nil {
		t.Fatal("closed store initializes")
	}
	if err := s.Verify([]byte("synthetic")); err == nil {
		t.Fatal("closed store verifies")
	}
	if _, err := s.SignIn([]byte("synthetic"), time.Now()); err == nil {
		t.Fatal("closed store signs in")
	}
	if _, err := s.Authenticate(strings.Repeat("A", 43), time.Now()); err == nil {
		t.Fatal("closed store authenticates")
	}
	if err := s.SignOut("invalid"); !errors.Is(err, ErrCredentials) {
		t.Fatal(err)
	}
	if _, err := sessionKey(strings.Repeat("!", 43)); !errors.Is(err, ErrCredentials) {
		t.Fatal(err)
	}
}

func TestSessionRejectsInvalidState(t *testing.T) {
	s := sessionStore(t)
	token, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	key, err := sessionKey(token)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []string{"bad", `{"version":1,"owner_id":"other","created":"2026-11-04T00:00:00Z","seen":"2026-11-04T00:00:00Z"}`} {
		if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(sessionBucket).Put(key, []byte(fixture)) }); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Authenticate(token, time.Date(2026, 11, 4, 0, 1, 0, 0, time.UTC)); err == nil {
			t.Fatal("invalid session authenticates")
		}
	}
}
