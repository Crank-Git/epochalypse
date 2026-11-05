package capsule

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

func publishedRecovery(t *testing.T, mode Mode) (*Store, Record) {
	t.Helper()
	s := capsuleStore(t, 32)
	input := capsuleInput(mode)
	input.ReleaseAt = time.Unix(1692806364, 0).UTC()
	now := input.ReleaseAt.Add(-time.Hour)
	record, err := s.Create(input, strings.NewReader("private text"), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bundle(record.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatal("draft exposes a bundle", err)
	}
	record, err = s.Publish(record.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	return s, record
}

func TestRecoveryAuthenticatesAllModes(t *testing.T) {
	for _, mode := range []Mode{Public, ServerHeld, Tlock} {
		t.Run(string(mode), func(t *testing.T) {
			s, record := publishedRecovery(t, mode)
			now := record.RequestedReleaseAt
			bundle, err := s.Bundle(record.ID, now)
			if err != nil {
				t.Fatal(err)
			}
			if mode == Tlock && bundle.ReleasedKey != "" {
				t.Fatal("tlock bundle retains a key")
			}
			encoded, err := json.Marshal(bundle)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte("private text")) {
				t.Fatal("bundle exposes plaintext")
			}
			decoded, err := DecodeBundle(bytes.NewReader(encoded), 32)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			plaintext, err := decoded.Open(context.Background(), fixtureBeacon(t), now)
			if err != nil || string(plaintext) != "private text" {
				t.Fatal("content differs", err)
			}
			if plaintext, err := decoded.Open(context.Background(), nil, time.Time{}); plaintext != nil || !errors.Is(err, ErrInput) {
				t.Fatal("invalid clock succeeds")
			}
			decoded.Record.ObjectSHA256 = strings.Repeat("0", 64)
			if plaintext, err := decoded.Open(context.Background(), fixtureBeacon(t), now); plaintext != nil || !errors.Is(err, ErrIntegrity) {
				t.Fatal("changed content exposes plaintext")
			}
		})
	}
}

func TestRecoveryBoundsUntrustedInput(t *testing.T) {
	maximum, err := EncodedObjectLimit(32)
	if err != nil {
		t.Fatal(err)
	}
	reader := &countedReader{Reader: strings.NewReader(strings.Repeat("x", maximum+metadataAllowance+1000))}
	if _, err := DecodeBundle(reader, 32); err == nil {
		t.Fatal("oversized envelope succeeds")
	}
	if reader.read != maximum+metadataAllowance+257 {
		t.Fatal("decoder reads beyond its allowance", reader.read)
	}
	if _, err := DecodeBundle(privateReaderFailure{}, 32); !errors.Is(err, ErrIntegrity) {
		t.Fatal("reader failure succeeds", err)
	}
	for _, limit := range []int{0, -1, int(^uint(0) >> 1)} {
		if _, err := DecodeBundle(strings.NewReader("{}"), limit); !errors.Is(err, ErrLimit) {
			t.Fatal("invalid limit succeeds", err)
		}
	}
	if _, err := DecodeBundle(nil, 32); !errors.Is(err, ErrLimit) {
		t.Fatal("absent input succeeds")
	}
}

func TestRecoveryBundleRejectsUnavailablePrivateState(t *testing.T) {
	s, record := publishedRecovery(t, ServerHeld)
	for _, value := range [][]byte{nil, make([]byte, 32)} {
		if err := s.db.Update(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte(installation.CapsuleKeysBucket))
			if value == nil {
				return bucket.Delete([]byte(record.ID))
			}
			return bucket.Put([]byte(record.ID), value)
		}); err != nil {
			t.Fatal(err)
		}
		bundle, err := s.Bundle(record.ID, record.RequestedReleaseAt)
		if err == nil || bundle.ReleasedKey != "" || bundle.Record.ID != "" {
			t.Fatal("invalid private state returns recovery data", err)
		}
	}
	if _, err := s.Bundle("invalid", record.RequestedReleaseAt); !errors.Is(err, ErrInput) {
		t.Fatal("invalid identifier succeeds")
	}
	if _, err := s.Bundle(record.ID, time.Time{}); !errors.Is(err, ErrInput) {
		t.Fatal("invalid clock succeeds")
	}
	if _, err := s.Bundle(strings.Repeat("a", 32), record.RequestedReleaseAt); !errors.Is(err, ErrNotFound) {
		t.Fatal("absent record succeeds")
	}
}

func TestRecoveryPreservesReleaseAndAvailability(t *testing.T) {
	for _, mode := range []Mode{ServerHeld, Tlock} {
		t.Run(string(mode), func(t *testing.T) {
			s, record := publishedRecovery(t, mode)
			early := record.RequestedReleaseAt.Add(-time.Second)
			bundle, err := s.Bundle(record.ID, early)
			if err != nil || bundle.ReleasedKey != "" {
				t.Fatal("early bundle exposes a key", err)
			}
			source := fixtureBeacon(t)
			if plaintext, err := bundle.Open(context.Background(), source, early); plaintext != nil || !errors.Is(err, ErrTooEarly) || source.calls != 0 {
				t.Fatal("early recovery succeeds or fetches a beacon")
			}
			if plaintext, err := bundle.Open(context.Background(), nil, record.RequestedReleaseAt); plaintext != nil || !errors.Is(err, ErrUnavailable) {
				t.Fatal("missing material does not delay availability", err)
			}
			if mode == Tlock {
				source.signature[0] ^= 1
				if plaintext, err := bundle.Open(context.Background(), source, record.RequestedReleaseAt); plaintext != nil || !errors.Is(err, ErrIntegrity) {
					t.Fatal("invalid signature exposes plaintext", err)
				}
				bundle.ReleasedKey = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
				if plaintext, err := bundle.Open(context.Background(), source, record.RequestedReleaseAt); plaintext != nil || !errors.Is(err, ErrIntegrity) {
					t.Fatal("tlock accepts an external key")
				}
			}
		})
	}
}

func TestRecoveryRejectsMismatchedAuthenticatedObject(t *testing.T) {
	s, record := publishedRecovery(t, Tlock)
	bundle, err := s.Bundle(record.ID, record.RequestedReleaseAt)
	if err != nil {
		t.Fatal(err)
	}
	object, _, err := SealObject(record.ID, FormatVersion, []byte("changed text"), 32)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Record.Object = object
	bundle.Record.ObjectSHA256 = objectDigest(object)
	if plaintext, err := bundle.Open(context.Background(), fixtureBeacon(t), record.RequestedReleaseAt); plaintext != nil || !errors.Is(err, ErrIntegrity) {
		t.Fatal("mismatched JWE exposes plaintext", err)
	}
}

func TestRecoveryRejectsMalformedBundles(t *testing.T) {
	s, record := publishedRecovery(t, Public)
	bundle, err := s.Bundle(record.ID, record.RequestedReleaseAt)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{nil, []byte(`{}`), append(bytes.Clone(encoded), []byte(`{}`)...), append([]byte(`{"version":1,`), encoded[1:]...), bytes.Replace(encoded, []byte(`"version":1`), []byte(`"version":2`), 1), bytes.Replace(encoded, []byte(`"version":1`), []byte(`"extra":1,"version":1`), 1)} {
		if _, err := DecodeBundle(bytes.NewReader(data), 32); !errors.Is(err, ErrIntegrity) {
			t.Fatal("malformed bundle succeeds", err)
		}
	}
	if _, err := DecodeBundle(bytes.NewReader(encoded), 1); err == nil {
		t.Fatal("oversized declared plaintext succeeds")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if plaintext, err := bundle.Open(ctx, nil, record.RequestedReleaseAt); plaintext != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled recovery succeeds", err)
	}
	bundle.ReleasedKey = "bad"
	if plaintext, err := bundle.Open(context.Background(), nil, record.RequestedReleaseAt); plaintext != nil || !errors.Is(err, ErrIntegrity) {
		t.Fatal("malformed released key succeeds", err)
	}
}
