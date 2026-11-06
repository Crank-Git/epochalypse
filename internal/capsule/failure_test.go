package capsule

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

func TestCapsuleOpenFailureReleasesTheInstallation(t *testing.T) {
	directory := t.TempDir()
	if _, err := Open(directory, installation.DefaultLimits(), 0); !errors.Is(err, ErrLimit) {
		t.Fatalf("invalid plaintext limit passes: %v", err)
	}
	shared, err := installation.Open(directory, installation.DefaultLimits())
	if err != nil {
		t.Fatalf("failed attachment retains the writer lock: %v", err)
	}
	if _, err := Attach(nil, 32); !errors.Is(err, ErrInput) {
		t.Fatalf("absent installation passes: %v", err)
	}
	s, err := Attach(shared, 32)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := shared.View(func(*bolt.Tx) error { return nil }); err != nil {
		t.Fatalf("capsule shutdown closes another domain: %v", err)
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Attach(shared, 32); err == nil {
		t.Fatal("a closed installation permits attachment")
	}
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("synthetic state blocker"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, installation.DefaultLimits(), 32); err == nil {
		t.Fatal("a regular file permits state access")
	}
}

func TestCapsuleContractsRejectInvalidAndMissingIdentifiers(t *testing.T) {
	s := capsuleStore(t, 32)
	for _, id := range []string{"invalid", strings.Repeat("a", 32)} {
		if _, err := s.Get(id); err == nil {
			t.Fatal("an invalid or absent record passes")
		}
		if _, err := s.Published(id); err == nil {
			t.Fatal("an invalid or absent publication passes")
		}
		if _, err := s.Publish(id, capsuleNow); err == nil {
			t.Fatal("an invalid or absent publication mutation passes")
		}
		if key, err := s.Key(id, capsuleNow); err == nil || key != [32]byte{} {
			t.Fatal("an invalid or absent key passes")
		}
	}
	if _, err := s.Publish(testID, time.Time{}); !errors.Is(err, ErrInput) {
		t.Fatalf("publication accepts an absent clock: %v", err)
	}
	if key, err := s.Key(testID, time.Time{}); !errors.Is(err, ErrInput) || key != [32]byte{} {
		t.Fatalf("key access accepts an absent clock: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("text"), capsuleNow); err == nil {
		t.Fatal("a closed store creates content")
	}
	if _, err := s.Get(testID); err == nil {
		t.Fatal("a closed store returns content")
	}
	if _, err := s.Publish(testID, capsuleNow); err == nil {
		t.Fatal("a closed store publishes content")
	}
	if key, err := s.Key(testID, capsuleNow); err == nil || key != [32]byte{} {
		t.Fatal("a closed store returns a key")
	}
}

type privateReaderFailure struct{}

func (privateReaderFailure) Read([]byte) (int, error) {
	return 0, errors.New("synthetic private reader content")
}

func TestCreateExcludesPrivateReaderErrors(t *testing.T) {
	s := capsuleStore(t, 32)
	if _, err := s.Create(capsuleInput(ServerHeld), privateReaderFailure{}, capsuleNow); !errors.Is(err, ErrInput) || strings.Contains(err.Error(), "synthetic private reader content") {
		t.Fatalf("reader failure exposes private content: %v", err)
	}
	if objects, keys := capsuleCounts(t, s); objects != 0 || keys != 0 {
		t.Fatal("reader failure retains partial state")
	}
}

func TestMissingPrivateStateBlocksCreationAndRelease(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(record.ID, capsuleNow); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket([]byte(installation.CapsuleKeysBucket)) }); err != nil {
		t.Fatal(err)
	}
	if key, err := s.Key(record.ID, record.RequestedReleaseAt); !errors.Is(err, ErrUnavailable) || key != [32]byte{} {
		t.Fatalf("a missing key bucket releases content: %v", err)
	}
	if _, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("text"), capsuleNow); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("a missing key bucket admits content: %v", err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket([]byte(installation.CapsulesBucket)) }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(record.ID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("a missing capsule bucket passes: %v", err)
	}
}

func TestMissingDraftKeyBlocksPublication(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(installation.CapsuleKeysBucket)).Put([]byte(record.ID), []byte("short synthetic key"))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(record.ID, capsuleNow); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("an invalid private key permits publication: %v", err)
	}
}

func TestCorruptReleasedKeyReturnsNoKey(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(record.ID, capsuleNow); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		key, err := keyAt(tx, record.ID)
		if err != nil {
			return err
		}
		key[0] ^= 1
		return tx.Bucket([]byte(installation.CapsuleKeysBucket)).Put([]byte(record.ID), key[:])
	}); err != nil {
		t.Fatal(err)
	}
	if key, err := s.Key(record.ID, record.RequestedReleaseAt); !errors.Is(err, ErrIntegrity) || key != [32]byte{} {
		t.Fatalf("a corrupt key reaches the caller: %v", err)
	}
}

func TestRecordRejectsUnsupportedFieldsAndExcessiveMetadata(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	data, err := encodeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		[]byte("not JSON"),
		append(append([]byte{}, data[:len(data)-1]...), []byte(`,"key":"synthetic private key"}`)...),
		append(append([]byte{}, data[:len(data)-1]...), []byte(`,"unknown":"`+strings.Repeat("x", metadataAllowance*2)+`"}`)...),
	} {
		if _, err := decodeRecord(record.ID, data); !errors.Is(err, ErrIntegrity) || strings.Contains(err.Error(), "synthetic private key") {
			t.Fatalf("unsupported record passes: %v", err)
		}
	}
	for _, size := range []int{int(^uint(0) >> 1), (int(^uint(0)>>1) - 1066) / 4 * 3} {
		record.PlaintextBytes = size
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeRecord(record.ID, data); !errors.Is(err, ErrIntegrity) {
			t.Fatalf("an excessive declared size passes: %v", err)
		}
	}
}

func TestPublicManifestRejectsFalsePublicationTimes(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(Public), strings.NewReader("text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Manifest){
		func(m *Manifest) { value := capsuleNow; m.EffectiveReleaseAt = &value },
		func(m *Manifest) { value := capsuleNow; m.PublishedAt = &value },
		func(m *Manifest) {
			value := capsuleNow.Add(-time.Second)
			m.PublishedAt = &value
			m.EffectiveReleaseAt = &value
		},
		func(m *Manifest) { value := time.Time{}; m.PublishedAt = &value },
		func(m *Manifest) {
			m.RequestedReleaseAt = m.RequestedReleaseAt.In(time.FixedZone("synthetic offset", 3600))
		},
	} {
		changed := record.Manifest
		change(&changed)
		if err := validateManifest(changed); !errors.Is(err, ErrIntegrity) {
			t.Fatalf("false public publication time passes: %v", err)
		}
	}
}

func TestRecordEncoderBoundsMetadataAndTime(t *testing.T) {
	if _, err := encodeRecord(Record{Manifest: Manifest{Title: strings.Repeat("x", metadataAllowance*2)}}); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("excessive encoded metadata passes: %v", err)
	}
	if _, err := encodeRecord(Record{Manifest: Manifest{CreatedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("unencodable time passes: %v", err)
	}
}

func TestCapsuleCountsUnopenedWindowState(t *testing.T) {
	s := capsuleStore(t, 32)
	s.owned.Limits.MaxObjects = 1
	data, err := os.ReadFile("../window/testdata/window-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(installation.WindowsBucket))
		if err != nil {
			return err
		}
		return bucket.Put([]byte("synthetic-window"), data)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("text"), capsuleNow); !errors.Is(err, installation.ErrQuota) {
		t.Fatalf("a capsule bypasses unopened windows: %v", err)
	}
	if objects, keys := capsuleCounts(t, s); objects != 0 || keys != 0 {
		t.Fatal("unopened-domain rejection retains a capsule")
	}
}
