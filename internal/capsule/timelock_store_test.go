package capsule

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

func TestTimelockDraftRetainsNoContentKey(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(Tlock), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if objects, keys := capsuleCounts(t, s); objects != 1 || keys != 1 {
		t.Fatal("draft lacks its record or preparation proof")
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		proof, err := keyAt(tx, record.ID)
		if err != nil {
			return err
		}
		if content, err := OpenObject(record.ID, record.Version, record.Object, proof, 32); err == nil || content != nil {
			t.Fatal("preparation proof exposes the content key")
		}
		if bytes.Contains(tx.Bucket([]byte(installation.CapsulesBucket)).Get([]byte(record.ID)), []byte("private text")) {
			t.Fatal("record exposes plaintext")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if key, err := s.Key(record.ID, capsuleNow.Add(2*time.Hour)); key != [32]byte{} || !errors.Is(err, ErrNotFound) {
		t.Fatal("draft exposes recovery material")
	}
}

func TestTimelockPublicationPreservesCeilingRoundAfterRestart(t *testing.T) {
	directory := t.TempDir()
	s, err := Open(directory, installation.DefaultLimits(), 32)
	if err != nil {
		t.Fatal(err)
	}
	input := capsuleInput(Tlock)
	input.ReleaseAt = time.Unix(1692806365, 0).UTC()
	now := input.ReleaseAt.Add(-time.Hour)
	record, err := s.Create(input, strings.NewReader("private text"), now)
	if err != nil {
		t.Fatal(err)
	}
	if record.EffectiveReleaseAt == nil || record.EffectiveReleaseAt.Unix() != 1692806367 {
		t.Fatal("draft does not select the ceiling round")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(directory, installation.DefaultLimits(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	published, err := s.Publish(record.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Publish(record.ID, now.Add(time.Second))
	if err != nil || !reflect.DeepEqual(published, again) {
		t.Fatal("publication changes immutable content")
	}
	if key, err := s.Key(record.ID, now); key != [32]byte{} || !errors.Is(err, ErrTooEarly) {
		t.Fatal("early key escapes")
	}
	if key, err := s.Key(record.ID, input.ReleaseAt.Add(time.Hour)); key != [32]byte{} || !errors.Is(err, ErrUnavailable) {
		t.Fatal("preparation proof escapes as a key")
	}
}

func TestTimelockPublicationRejectsChangedDraft(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(Tlock), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	record.Title = "Changed title"
	data, err := encodeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(installation.CapsulesBucket)).Put([]byte(record.ID), data)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(record.ID, capsuleNow); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("changed draft publishes: %v", err)
	}
	if _, err := s.Published(record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("failed publication exposes draft")
	}
}

func TestTimelockWrapperConsumesSharedBudget(t *testing.T) {
	s := capsuleStore(t, 32)
	s.owned.Limits.MaxBytes = 1000
	if _, err := s.Create(capsuleInput(Tlock), strings.NewReader("text"), capsuleNow); !errors.Is(err, installation.ErrQuota) {
		t.Fatalf("wrapper bypasses quota: %v", err)
	}
	if objects, keys := capsuleCounts(t, s); objects != 0 || keys != 0 {
		t.Fatal("quota failure retains state")
	}
}
