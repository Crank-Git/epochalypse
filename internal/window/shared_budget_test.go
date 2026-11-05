package window

import (
	"errors"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestWindowAdmissionCountsUnopenedCapsuleRecords(t *testing.T) {
	s, err := OpenWithLimits(t.TempDir(), testSigner(t), Limits{MaxObjects: 1, MaxBytes: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("capsules-v1"))
		if err != nil {
			return err
		}
		return bucket.Put([]byte("0123456789abcdef0123456789abcdef"), []byte(`{"version":1}`))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(validInput(), testNow); !errors.Is(err, ErrQuota) {
		t.Fatalf("unopened capsules bypass the installation count: %v", err)
	}
	windows, err := s.List()
	if err != nil || len(windows) != 0 {
		t.Fatalf("quota failure changes windows: %v", err)
	}
}

func TestWindowAdmissionCountsSeparateCapsuleKeyBytes(t *testing.T) {
	s, err := OpenWithLimits(t.TempDir(), testSigner(t), Limits{MaxObjects: 100, MaxBytes: 32})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("capsule-keys-v1"))
		if err != nil {
			return err
		}
		return bucket.Put([]byte("0123456789abcdef0123456789abcdef"), make([]byte, 32))
	}); err != nil {
		t.Fatal(err)
	}
	// A zero-size probe isolates retained key bytes from the new window document.
	if err := s.db.Update(func(tx *bolt.Tx) error { return s.admits(tx, 1) }); !errors.Is(err, ErrQuota) {
		t.Fatalf("private keys bypass the installation bytes: %v", err)
	}
}
