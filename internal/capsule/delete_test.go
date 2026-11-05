package capsule

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

func TestDeletePreservesIndependentRecovery(t *testing.T) {
	for _, mode := range []Mode{Public, ServerHeld, Tlock} {
		t.Run(string(mode), func(t *testing.T) {
			s, record := publishedRecovery(t, mode)
			bundle, err := s.Bundle(record.ID, record.RequestedReleaseAt)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(bundle)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Delete(context.Background(), record.ID, record.ID); err != nil {
				t.Fatal(err)
			}
			if objects, keys := capsuleCounts(t, s); objects != 0 || keys != 0 {
				t.Fatal("local content remains")
			}
			if _, err := s.Bundle(record.ID, record.RequestedReleaseAt); !errors.Is(err, ErrNotFound) {
				t.Fatal("deleted capsule remains public", err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			retained, err := DecodeBundle(bytes.NewReader(data), 32)
			if err != nil {
				t.Fatal(err)
			}
			plaintext, err := retained.Open(context.Background(), fixtureBeacon(t), record.RequestedReleaseAt)
			if err != nil || string(plaintext) != "private text" {
				t.Fatal("retained public copy cannot recover", err)
			}
		})
	}
}

func TestDeleteRequiresExactConfirmation(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("secret"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, confirmation := range []string{"", "yes", record.ID + " ", strings.ToUpper(record.ID)} {
		if err := s.Delete(context.Background(), record.ID, confirmation); !errors.Is(err, ErrInput) {
			t.Fatal("invalid confirmation passes", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Delete(ctx, record.ID, record.ID); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled deletion passes", err)
	}
	if objects, keys := capsuleCounts(t, s); objects != 1 || keys != 1 {
		t.Fatal("failed confirmation changes state")
	}
	if err := s.Delete(context.Background(), "invalid", "invalid"); !errors.Is(err, ErrInput) {
		t.Fatal("invalid identifier passes", err)
	}
	if err := s.Delete(context.Background(), testID, testID); !errors.Is(err, ErrNotFound) {
		t.Fatal("absent identifier passes", err)
	}
	if err := s.Delete(context.Background(), record.ID, record.ID); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteReleasesSharedQuota(t *testing.T) {
	s, err := Open(t.TempDir(), installation.Limits{MaxObjects: 2, MaxBytes: 1 << 20}, 32)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("secret"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		// A window charge proves deletion preserves another retained domain.
		bucket, err := tx.CreateBucketIfNotExists([]byte(installation.WindowsBucket))
		if err != nil {
			return err
		}
		return bucket.Put([]byte("retained-window"), []byte(`{"version":1}`))
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error { return s.db.Admit(tx, 1, 32) }); !errors.Is(err, installation.ErrQuota) {
		t.Fatal("full installation admits another object", err)
	}
	if err := s.Delete(context.Background(), record.ID, record.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		if string(tx.Bucket([]byte(installation.WindowsBucket)).Get([]byte("retained-window"))) != `{"version":1}` {
			t.Fatal("deletion changes another domain")
		}
		return s.db.Admit(tx, 1, 32)
	}); err != nil {
		t.Fatal("deletion retains a budget charge", err)
	}
}

type deletionCancellation struct {
	context.Context
	calls    int
	cancelAt int
}

func (c *deletionCancellation) Err() error {
	c.calls++
	if c.calls >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestDeleteRollsBackCancellationBeforeCommit(t *testing.T) {
	for _, cancelAt := range []int{2, 3} {
		s := capsuleStore(t, 32)
		record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("secret"), capsuleNow)
		if err != nil {
			t.Fatal(err)
		}
		ctx := &deletionCancellation{Context: context.Background(), cancelAt: cancelAt}
		if err := s.Delete(ctx, record.ID, record.ID); !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation passes", err)
		}
		if objects, keys := capsuleCounts(t, s); objects != 1 || keys != 1 {
			t.Fatal("canceled transaction retains partial state")
		}
	}
}
