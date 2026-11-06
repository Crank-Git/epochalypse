package switches

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func advanceRenewalProof(t *testing.T, prior RenewalEvidence, key ed25519.PrivateKey, keys [3]ed25519.PrivateKey) RenewalEvidence {
	t.Helper()
	f, err := switchwire.DecodeFrame(prior.Request.Body)
	if err != nil {
		t.Fatal(err)
	}
	f.State.Sequence++
	f.State.Deadline++
	request, err := switchwire.SignFrame(f, key)
	if err != nil {
		t.Fatal(err)
	}
	p := RenewalEvidence{ConfirmedRequest: prior.Request, ConfirmedReceipts: prior.Receipts, Request: request}
	for i, k := range keys {
		r, err := switchwire.SignFrame(switchwire.Frame{Operation: switchwire.OperationReceipt, Signer: fmt.Sprintf("agent%d", i), State: f.State, Durable: f.State.Sequence, Phase: switchwire.PhaseArmed}, k)
		if err != nil {
			t.Fatal(err)
		}
		p.Receipts = append(p.Receipts, r)
	}
	return p
}

func TestRenewalReconcilesAfterMainMissesConfirmedSequences(t *testing.T) {
	_, store := newDraftStore(t, t.TempDir())
	draft, err := store.Create(context.Background(), Input{}, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	arm, key, keys := armEvidenceKeys(t, draft)
	if _, err := store.ObserveArm(context.Background(), draft.ID, arm); err != nil {
		t.Fatal(err)
	}
	second := signedRenewalProof(t, arm, key, keys)
	partial := second
	partial.Receipts = second.Receipts[:1]
	if _, err := store.ObserveRenewal(context.Background(), draft.ID, partial); err != nil {
		t.Fatal(err)
	}
	third := advanceRenewalProof(t, second, key, keys)
	partial = third
	partial.Receipts = third.Receipts[:2]
	r, err := store.ObserveRenewal(context.Background(), draft.ID, partial)
	if err != nil || r.RenewalStatus().Bmax-r.RenewalStatus().B0 != 1 {
		t.Fatalf("successor evidence: %+v %v", r, err)
	}
	if _, err := store.ObserveRenewal(context.Background(), draft.ID, second); !errors.Is(err, ErrInput) {
		t.Fatalf("old sequence: %v", err)
	}
	conflict := advanceRenewalProof(t, second, key, keys)
	f, _ := switchwire.DecodeFrame(conflict.Request.Body)
	f.State.Deadline++
	conflict.Request, err = switchwire.SignFrame(f, key)
	if err != nil {
		t.Fatal(err)
	}
	conflict.Receipts = nil
	if _, err := store.ObserveRenewal(context.Background(), draft.ID, conflict); !errors.Is(err, ErrInput) {
		t.Fatalf("same sequence changes intent: %v", err)
	}
	fourth := advanceRenewalProof(t, third, key, keys)
	fifth := advanceRenewalProof(t, fourth, key, keys)
	fifth.Receipts = fifth.Receipts[:2]
	r, err = store.ObserveRenewal(context.Background(), draft.ID, fifth)
	if err != nil || r.State != StateUncertain || r.RenewalStatus().Bmax-r.RenewalStatus().B0 != 1 {
		t.Fatalf("offline main reconciliation: %+v %v", r, err)
	}
	r, err = store.ObserveArm(context.Background(), draft.ID, arm)
	if err != nil || r.State != StateUncertain {
		t.Fatalf("arm replay hides uncertain renewal: %+v %v", r, err)
	}
}

func TestRenewalObservationPreservesStateAfterAdmissionFailure(t *testing.T) {
	shared, store := newDraftStore(t, t.TempDir())
	draft, err := store.Create(context.Background(), Input{}, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	arm, key, keys := armEvidenceKeys(t, draft)
	before, err := store.ObserveArm(context.Background(), draft.ID, arm)
	if err != nil {
		t.Fatal(err)
	}
	p := signedRenewalProof(t, arm, key, keys)
	data, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	shared.Limits.MaxBytes = int64(len(data) + 1)
	if _, err := store.ObserveRenewal(context.Background(), draft.ID, p); !errors.Is(err, installation.ErrQuota) {
		t.Fatalf("quota: %v", err)
	}
	after, err := store.Get(draft.ID)
	if err != nil || after.Renewal != nil || after.State != StateArmed {
		t.Fatalf("failed admission changes state: %+v %v", after, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ObserveRenewal(ctx, draft.ID, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled observation: %v", err)
	}
	if _, err := store.ObserveRenewal(context.Background(), "invalid", p); !errors.Is(err, ErrInput) {
		t.Fatalf("invalid identifier: %v", err)
	}
	if _, err := store.ObserveRenewal(context.Background(), "0123456789abcdef0123456789abcdef", p); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown switch: %v", err)
	}
}
