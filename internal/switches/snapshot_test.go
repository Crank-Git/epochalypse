package switches

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

func TestSnapshotValidatesDraftAndArmEvidenceWithoutMutation(t *testing.T) {
	shared, store := newDraftStore(t, t.TempDir())
	draft, err := store.Create(context.Background(), Input{}, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	evidence := armEvidence(t, draft)
	for _, count := range []int{-1, 0, 1, 3} {
		record := draft
		if count >= 0 {
			partial := evidence
			partial.Armed = evidence.Armed[:count]
			record.Arm, record.State = &partial, evidenceState(partial)
		}
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := shared.Update(func(tx *bolt.Tx) error {
			return tx.Bucket([]byte(installation.SwitchesBucket)).Put([]byte(record.ID), data)
		}); err != nil {
			t.Fatal(err)
		}
		if err := shared.View(ValidateSnapshot); err != nil {
			t.Fatalf("receipt count %d: %v", count, err)
		}
		if err := shared.View(func(tx *bolt.Tx) error {
			if string(tx.Bucket([]byte(installation.SwitchesBucket)).Get([]byte(record.ID))) != string(data) {
				t.Fatal("snapshot validator changes retained bytes")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	evidence.Armed[0].Signature[0] ^= 1
	record := draft
	record.State, record.Arm = StateArmed, &evidence
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := shared.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(installation.SwitchesBucket)).Put([]byte(record.ID), data)
	}); err != nil {
		t.Fatal(err)
	}
	if err := shared.View(ValidateSnapshot); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("snapshot accepts corrupt arm evidence: %v", err)
	}
}

func TestSnapshotAcceptsOlderInstallationsAndRejectsMissingTransaction(t *testing.T) {
	if err := ValidateSnapshot(nil); !errors.Is(err, ErrInput) {
		t.Fatalf("missing transaction: %v", err)
	}
	shared, err := installation.Open(t.TempDir(), installation.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shared.Close() }()
	if err := shared.View(ValidateSnapshot); err != nil {
		t.Fatal(err)
	}
}
