package owner

import (
	"bytes"
	"encoding/json"
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

func TestSnapshotRejectsIncompatibleOwnerRecords(t *testing.T) {
	cases := map[string]func(*bolt.Tx) error{
		"missing bucket":  func(tx *bolt.Tx) error { return tx.DeleteBucket(sessionBucket) },
		"unknown bucket":  func(tx *bolt.Tx) error { _, err := tx.CreateBucket([]byte("owner-v2")); return err },
		"extra identity":  func(tx *bolt.Tx) error { return tx.Bucket(stateBucket).Put([]byte("extra"), []byte(`{}`)) },
		"nested bucket":   func(tx *bolt.Tx) error { _, err := tx.Bucket(tokenBucket).CreateBucket([]byte("nested")); return err },
		"session version": func(tx *bolt.Tx) error { return changeSnapshotRecord(tx, sessionBucket, "version", 2) },
		"session owner":   func(tx *bolt.Tx) error { return changeSnapshotRecord(tx, sessionBucket, "owner_id", "other") },
		"session time": func(tx *bolt.Tx) error {
			return changeSnapshotRecord(tx, sessionBucket, "seen", tokenClock.Add(-time.Second))
		},
		"session key":   func(tx *bolt.Tx) error { return changeSnapshotKey(tx, sessionBucket, []byte("wrong")) },
		"token owner":   func(tx *bolt.Tx) error { return changeSnapshotRecord(tx, tokenBucket, "owner_id", "other") },
		"token digest":  func(tx *bolt.Tx) error { return changeSnapshotRecord(tx, tokenBucket, "digest", "wrong") },
		"token scope":   func(tx *bolt.Tx) error { return changeSnapshotRecord(tx, tokenBucket, "scopes", []string{"admin"}) },
		"audit action":  func(tx *bolt.Tx) error { return changeSnapshotRecord(tx, auditBucket, "action", "unknown") },
		"audit key":     func(tx *bolt.Tx) error { return changeSnapshotKey(tx, auditBucket, []byte("wrong")) },
		"unknown field": func(tx *bolt.Tx) error { return changeSnapshotRecord(tx, stateBucket, "future", true) },
		"invalid JSON": func(tx *bolt.Tx) error {
			return tx.Bucket(sessionBucket).Put(bytes.Repeat([]byte{1}, 32), []byte(`secret-invalid`))
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			path := ownerSnapshot(t)
			db, err := bolt.Open(path, 0600, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Update(mutate); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := validateOwnerSnapshot(t, path); err == nil {
				t.Fatal("snapshot accepts incompatible owner records")
			} else if bytes.Contains([]byte(err.Error()), []byte("secret-invalid")) {
				t.Fatal("snapshot error exposes stored content")
			}
		})
	}
}

func changeSnapshotRecord(tx *bolt.Tx, name []byte, field string, value any) error {
	b := tx.Bucket(name)
	key, data := b.Cursor().First()
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	record[field] = value
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return b.Put(key, encoded)
}

func changeSnapshotKey(tx *bolt.Tx, name, replacement []byte) error {
	b := tx.Bucket(name)
	key, data := b.Cursor().First()
	copy := bytes.Clone(data)
	if err := b.Delete(key); err != nil {
		return err
	}
	return b.Put(replacement, copy)
}

func TestSnapshotRecordValidatorsRejectMalformedDocuments(t *testing.T) {
	for _, data := range [][]byte{nil, bytes.Repeat([]byte("x"), 16385), []byte(`{} {}`)} {
		if err := snapshotDocument(data, &session{}); err == nil {
			t.Fatal("snapshot accepts an invalid record document")
		}
	}
	if err := validateSnapshotToken(nil, []byte(`invalid`), "owner"); err == nil {
		t.Fatal("snapshot accepts invalid token JSON")
	}
	if err := validateSnapshotAudit(nil, []byte(`invalid`), 0); err == nil {
		t.Fatal("snapshot accepts invalid audit JSON")
	}
}

func TestSnapshotPreservesExpiredCredentials(t *testing.T) {
	path := ownerSnapshot(t)
	if err := validateOwnerSnapshot(t, path); err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Dir(filepath.Dir(path)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.db.View(func(tx *bolt.Tx) error {
		_, data := tx.Bucket(sessionBucket).Cursor().First()
		var record session
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		if record.valid(tokenClock.Add(12 * time.Hour)) {
			t.Fatal("snapshot extends session validity")
		}
		_, data = tx.Bucket(tokenBucket).Cursor().First()
		var token tokenRecord
		if err := json.Unmarshal(data, &token); err != nil {
			return err
		}
		if token.valid(token.OwnerID, token.ExpiresAt) {
			t.Fatal("snapshot extends token validity")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
