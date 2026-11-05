package window

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestStorePreservesSignedRecordsAfterRestart(t *testing.T) {
	state, signer := t.TempDir(), testSigner(t)
	store, err := Open(state, signer)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("creation does not return a durable window")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(state, signer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	got, err := restarted.Get(created.ID)
	if err != nil || !reflect.DeepEqual(got, created) {
		t.Fatalf("restart changes the window: %v", err)
	}
	if _, _, _, err := got.Records(); err != nil {
		t.Fatal(err)
	}
	all, err := restarted.List()
	if err != nil || len(all) != 1 || !reflect.DeepEqual(all[0], created) {
		t.Fatalf("list changes the window: %v", err)
	}
	if _, err := restarted.Get("../../owner.db"); err == nil {
		t.Fatal("invalid identifier passes")
	}
	if _, err := restarted.Get("00000000000000000000000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(state, "v1", "windows.db")); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("window database lacks owner-only access")
	}
}

func TestStoreRejectsInvalidCreation(t *testing.T) {
	s, err := Open(t.TempDir(), testSigner(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	in := validInput()
	in.Content = strings.Repeat("a", 513)
	if _, err := s.Create(in, testNow); err == nil {
		t.Fatal("store accepts invalid content")
	}
	all, err := s.List()
	if err != nil || len(all) != 0 {
		t.Fatal("invalid creation changes stored records")
	}
}

func TestStoreRefusesASecondWriter(t *testing.T) {
	state, signer := t.TempDir(), testSigner(t)
	s, err := Open(state, signer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if other, err := Open(state, signer); err == nil {
		_ = other.Close()
		t.Fatal("second writer opens the locked database")
	}
}

func TestSnapshotValidationChecksWindowAndObservationRecords(t *testing.T) {
	s, err := Open(t.TempDir(), testSigner(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Create(validInput(), testNow); err != nil {
		t.Fatal(err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error { return ValidateSnapshot(tx, s.signer, *s.limits) }); err != nil {
		t.Fatalf("valid window snapshot: %v", err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(windowBucket).Put([]byte("invalid"), []byte(`{"version":2}`))
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error { return ValidateSnapshot(tx, s.signer, *s.limits) }); err == nil {
		t.Fatal("snapshot accepts an invalid window")
	}
}

func TestSnapshotValidationRejectsMissingDNSKEYConfiguration(t *testing.T) {
	s, err := Open(t.TempDir(), testSigner(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(configurationBucket).Delete([]byte("dnskey"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error { return ValidateSnapshot(tx, s.signer, *s.limits) }); err == nil {
		t.Fatal("snapshot accepts missing DNSKEY configuration")
	}
}
