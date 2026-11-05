package capsule

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

var capsuleNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func capsuleInput(mode Mode) Input {
	return Input{Mode: mode, Kind: Text, Title: "Synthetic capsule", ReleaseAt: capsuleNow.Add(time.Hour)}
}

func capsuleStore(t *testing.T, limit int) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), installation.DefaultLimits(), limit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestServerHeldDraftWithholdsObjectAndKeyUntilPublication(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Published(record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("draft exposes an object: %v", err)
	}
	if key, err := s.Key(record.ID, capsuleNow.Add(2*time.Hour)); !errors.Is(err, ErrNotFound) || key != [32]byte{} {
		t.Fatalf("draft exposes a key: %v", err)
	}
}

func TestServerHeldKeyUsesExactReleaseBoundary(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	published, err := s.Publish(record.ID, capsuleNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if published.EffectiveReleaseAt == nil || !published.EffectiveReleaseAt.Equal(record.RequestedReleaseAt) {
		t.Fatal("server-held publication changes its release time")
	}
	if key, err := s.Key(record.ID, record.RequestedReleaseAt.Add(-time.Nanosecond)); !errors.Is(err, ErrTooEarly) || key != [32]byte{} {
		t.Fatalf("early key: %v", err)
	}
	key, err := s.Key(record.ID, record.RequestedReleaseAt)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := OpenObject(record.ID, record.Version, published.Object, key, 32)
	if err != nil || string(plaintext) != "private text" {
		t.Fatalf("released key cannot authenticate content: %v", err)
	}
}

func TestPublicModeExposesKeyAtPublication(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(Public), strings.NewReader("public text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if record.EffectiveReleaseAt != nil {
		t.Fatal("a private public-mode draft claims an effective publication time")
	}
	publishedAt := capsuleNow.Add(time.Minute)
	published, err := s.Publish(record.ID, publishedAt)
	if err != nil {
		t.Fatal(err)
	}
	if published.EffectiveReleaseAt == nil || !published.EffectiveReleaseAt.Equal(publishedAt) || !published.RequestedReleaseAt.Equal(record.RequestedReleaseAt) {
		t.Fatal("public publication loses its actual or requested time")
	}
	key, err := s.Key(record.ID, publishedAt)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := OpenObject(record.ID, record.Version, published.Object, key, 32)
	if err != nil || string(plaintext) != "public text" {
		t.Fatalf("public key cannot authenticate content: %v", err)
	}
}

func TestCreateReadsAtMostOneByteBeyondPlaintextLimit(t *testing.T) {
	s := capsuleStore(t, 4)
	reader := &countedReader{Reader: strings.NewReader(strings.Repeat("x", 10000))}
	if _, err := s.Create(capsuleInput(ServerHeld), reader, capsuleNow); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversized content passes: %v", err)
	}
	if reader.read != 5 {
		t.Fatalf("the reader consumes %d bytes instead of limit plus one", reader.read)
	}
	if _, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("1234"), capsuleNow); err != nil {
		t.Fatalf("exact limit fails: %v", err)
	}
}

type countedReader struct {
	io.Reader
	read int
}

func (r *countedReader) Read(data []byte) (int, error) {
	n, err := r.Reader.Read(data)
	r.read += n
	return n, err
}

func TestDraftStoresFreshKeysOutsidePublicMetadata(t *testing.T) {
	s := capsuleStore(t, 32)
	first, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.Object == second.Object {
		t.Fatal("separate drafts reuse an identifier or object")
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		key, err := keyAt(tx, first.ID)
		if err != nil {
			return err
		}
		other, err := keyAt(tx, second.ID)
		if err != nil {
			return err
		}
		if key == other {
			t.Fatal("separate drafts reuse a key")
		}
		data := tx.Bucket([]byte(installation.CapsulesBucket)).Get([]byte(first.ID))
		if bytes.Contains(data, []byte("private text")) || bytes.Contains(data, []byte(`"key"`)) {
			t.Fatal("the public record includes plaintext or a key")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRejectsInvalidMetadataAndText(t *testing.T) {
	s := capsuleStore(t, 32)
	for _, change := range []func(*Input){
		func(in *Input) { in.Mode = Tlock },
		func(in *Input) { in.Mode = "unknown" },
		func(in *Input) { in.Kind = "unknown" },
		func(in *Input) { in.Title = strings.Repeat("x", 257) },
		func(in *Input) { in.Title = "line\nbreak" },
		func(in *Input) { in.Filename = strings.Repeat("x", 256) },
		func(in *Input) { in.Filename = "../content" },
		func(in *Input) { in.Filename = `C:\content` },
		func(in *Input) { in.ReleaseAt = capsuleNow.Add(-time.Second) },
		func(in *Input) { in.ReleaseAt = time.Time{} },
		func(in *Input) { in.ReleaseAt = in.ReleaseAt.Add(time.Nanosecond) },
	} {
		input := capsuleInput(ServerHeld)
		change(&input)
		reader := &countedReader{Reader: strings.NewReader("content")}
		if _, err := s.Create(input, reader, capsuleNow); err == nil {
			t.Fatalf("invalid metadata passes: %+v", input)
		}
		if reader.read != 0 {
			t.Fatal("invalid metadata consumes content")
		}
	}
	if _, err := s.Create(capsuleInput(ServerHeld), strings.NewReader(string([]byte{255})), capsuleNow); !errors.Is(err, ErrInput) {
		t.Fatalf("invalid UTF-8 text passes: %v", err)
	}
	input := capsuleInput(ServerHeld)
	input.Kind = File
	if _, err := s.Create(input, bytes.NewReader([]byte{255}), capsuleNow); err != nil {
		t.Fatalf("opaque file fails: %v", err)
	}
	if _, err := s.Create(capsuleInput(ServerHeld), nil, capsuleNow); !errors.Is(err, ErrInput) {
		t.Fatalf("absent content passes: %v", err)
	}
	if _, err := s.Create(capsuleInput(ServerHeld), strings.NewReader(""), time.Time{}); !errors.Is(err, ErrInput) {
		t.Fatalf("absent clock passes: %v", err)
	}
}

func TestPublicationPreservesIdempotentTimes(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(Public), strings.NewReader("text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(record.ID, capsuleNow.Add(-time.Second)); !errors.Is(err, ErrInput) {
		t.Fatalf("publication precedes creation: %v", err)
	}
	first, err := s.Publish(record.ID, capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Publish(record.ID, capsuleNow.Add(2*time.Hour))
	if err != nil || !reflect.DeepEqual(first, again) {
		t.Fatalf("repeated publication changes metadata: %v", err)
	}
	*again.EffectiveReleaseAt = capsuleNow.Add(5 * time.Hour)
	stored, err := s.Published(record.ID)
	if err != nil || !stored.EffectiveReleaseAt.Equal(capsuleNow) {
		t.Fatalf("caller mutation changes stored publication: %v", err)
	}
}

func capsuleCounts(t *testing.T, s *Store) (int, int) {
	t.Helper()
	objects, keys := 0, 0
	if err := s.db.View(func(tx *bolt.Tx) error {
		objects = tx.Bucket([]byte(installation.CapsulesBucket)).Stats().KeyN
		keys = tx.Bucket([]byte(installation.CapsuleKeysBucket)).Stats().KeyN
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return objects, keys
}

func TestCreateAndPublishRejectQuotaWithoutPartialState(t *testing.T) {
	s := capsuleStore(t, 32)
	s.owned.Limits.MaxBytes = 1
	if _, err := s.Create(capsuleInput(Public), strings.NewReader("text"), capsuleNow); !errors.Is(err, installation.ErrQuota) {
		t.Fatalf("quota failure: %v", err)
	}
	if objects, keys := capsuleCounts(t, s); objects != 0 || keys != 0 {
		t.Fatal("failed creation retains partial state")
	}
	s.owned.Limits = installation.DefaultLimits()
	record, err := s.Create(capsuleInput(Public), strings.NewReader("text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	data, err := encodeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	s.owned.Limits.MaxBytes = int64(len(data) + 32)
	if _, err := s.Publish(record.ID, capsuleNow); !errors.Is(err, installation.ErrQuota) {
		t.Fatalf("publication ignores its larger record: %v", err)
	}
	if _, err := s.Published(record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed publication exposes its draft: %v", err)
	}
	if key, err := s.Key(record.ID, record.RequestedReleaseAt); !errors.Is(err, ErrNotFound) || key != [32]byte{} {
		t.Fatal("failed publication exposes its key")
	}
	if objects, keys := capsuleCounts(t, s); objects != 1 || keys != 1 {
		t.Fatal("failed publication loses retained state")
	}
	s.owned.Limits = installation.DefaultLimits()
	if _, err := s.Publish(record.ID, capsuleNow); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentCreationSharesOneAtomicCapsuleBudget(t *testing.T) {
	s := capsuleStore(t, 32)
	s.owned.Limits.MaxObjects = 1
	results := make(chan error, 8)
	for range 8 {
		go func() {
			_, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("text"), capsuleNow)
			results <- err
		}()
	}
	accepted, rejected := 0, 0
	for range 8 {
		err := <-results
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, installation.ErrQuota):
			rejected++
		default:
			t.Fatal(err)
		}
	}
	if accepted != 1 || rejected != 7 {
		t.Fatalf("creation accepts %d and rejects %d", accepted, rejected)
	}
	if objects, keys := capsuleCounts(t, s); objects != 1 || keys != 1 {
		t.Fatal("concurrent quota failures leave partial state")
	}
}

func TestCapsuleRestartPreservesKeysAfterUploadLimitDecrease(t *testing.T) {
	directory := t.TempDir()
	s, err := Open(directory, installation.DefaultLimits(), 32)
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(record.ID, capsuleNow); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(directory, installation.DefaultLimits(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	stored, err := s.Published(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.Key(record.ID, record.RequestedReleaseAt)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := OpenObject(record.ID, record.Version, stored.Object, key, 32)
	if err != nil || string(plaintext) != "private text" {
		t.Fatalf("restart changes retained content: %v", err)
	}
	if _, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("new text"), capsuleNow); !errors.Is(err, ErrLimit) {
		t.Fatalf("restart ignores the new upload limit: %v", err)
	}
}

func TestKeyAvailabilityDoesNotOverrideTheReleaseBoundary(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(record.ID, capsuleNow); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(installation.CapsuleKeysBucket)).Delete([]byte(record.ID))
	}); err != nil {
		t.Fatal(err)
	}
	if key, err := s.Key(record.ID, capsuleNow); !errors.Is(err, ErrTooEarly) || key != [32]byte{} {
		t.Fatalf("missing key changes the early boundary: %v", err)
	}
	if key, err := s.Key(record.ID, record.RequestedReleaseAt); !errors.Is(err, ErrUnavailable) || key != [32]byte{} {
		t.Fatalf("missing key appears available: %v", err)
	}
	if _, err := s.Published(record.ID); err != nil {
		t.Fatalf("missing key hides release metadata: %v", err)
	}
}

func TestCorruptPrivateKeyCannotPublishOrReleaseContent(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("private text"), capsuleNow)
	if err != nil {
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
	if _, err := s.Publish(record.ID, capsuleNow); !errors.Is(err, ErrIntegrity) || strings.Contains(err.Error(), "private text") {
		t.Fatalf("corrupt key publishes content: %v", err)
	}
	if _, err := s.Published(record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("corrupt draft becomes public: %v", err)
	}
}

func TestRecordRejectsCorruptManifestAndObject(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Record){
		func(r *Record) { r.Version = 2 },
		func(r *Record) { r.ID = strings.Repeat("a", 32) },
		func(r *Record) { r.Mode = Tlock },
		func(r *Record) { r.Kind = "unknown" },
		func(r *Record) { r.Title = strings.Repeat("x", 257) },
		func(r *Record) { r.Filename = "" },
		func(r *Record) { r.CreatedAt = time.Time{} },
		func(r *Record) { r.EffectiveReleaseAt = nil },
		func(r *Record) { r.PlaintextBytes = -1 },
		func(r *Record) { r.PlaintextBytes++ },
		func(r *Record) { r.ObjectSHA256 = strings.Repeat("0", 64) },
		func(r *Record) { r.Object += "."; r.ObjectSHA256 = objectDigest(r.Object) },
	} {
		changed := record
		change(&changed)
		data, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket([]byte(installation.CapsulesBucket)).Put([]byte(record.ID), data)
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(record.ID); !errors.Is(err, ErrIntegrity) {
			t.Fatalf("corrupt record passes: %v", err)
		}
	}
}

func TestVersionOneRecordFixturePreservesReleaseMetadata(t *testing.T) {
	data, err := os.ReadFile("testdata/record-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	record, err := decodeRecord(testID, data)
	if err != nil {
		t.Fatal(err)
	}
	if record.Mode != ServerHeld || record.EffectiveReleaseAt == nil || !record.EffectiveReleaseAt.Equal(capsuleNow.Add(time.Hour)) || record.PublishedAt == nil || !record.PublishedAt.Equal(capsuleNow) {
		t.Fatal("the version 1 fixture loses release metadata")
	}
	encoded, err := encodeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	again, err := decodeRecord(testID, encoded)
	if err != nil || !reflect.DeepEqual(record, again) {
		t.Fatalf("the version 1 record changes: %v", err)
	}
	// The disclosed synthetic key matches the authenticated object fixture.
	var key [32]byte
	for i := range key {
		key[i] = byte(i)
	}
	plaintext, err := OpenObject(record.ID, record.Version, record.Object, key, 32)
	if err != nil || string(plaintext) != "synthetic capsule fixture" {
		t.Fatalf("the record fixture cannot authenticate content: %v", err)
	}
}
