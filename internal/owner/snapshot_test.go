package owner

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func ownerSnapshot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize([]byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignIn([]byte("synthetic"), tokenClock); err != nil {
		t.Fatal(err)
	}
	createTestToken(t, s, "read")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "v1", "owner.db")
}

func validateOwnerSnapshot(t *testing.T, path string) error {
	t.Helper()
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	return db.View(ValidateSnapshot)
}

func TestSnapshotAcceptsOwnerStateWithoutMutation(t *testing.T) {
	path := ownerSnapshot(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOwnerSnapshot(t, path); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("snapshot validation changes owner state")
	}
}

func TestSnapshotRejectsInvalidOwnerIdentity(t *testing.T) {
	path := ownerSnapshot(t)
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(stateBucket).Put(identityKey, []byte(`{"version":2}`))
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validateOwnerSnapshot(t, path); err == nil {
		t.Fatal("snapshot accepts an invalid owner identity")
	}
}
