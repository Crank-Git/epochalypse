package switches

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestRenewalEvidenceDerivesBoundsAndPreservesReceipts(t *testing.T) {
	_, store := newDraftStore(t, t.TempDir())
	draft, err := store.Create(context.Background(), Input{}, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	arm, key, keys := armEvidenceKeys(t, draft)
	if _, err := store.ObserveArm(context.Background(), draft.ID, arm); err != nil {
		t.Fatal(err)
	}
	e := signedRenewalProof(t, arm, key, keys)
	testRenewalProofRetention(t, store, draft, arm, e)
}

func signedRenewalProof(t *testing.T, arm ArmEvidence, key ed25519.PrivateKey, keys [3]ed25519.PrivateKey) RenewalEvidence {
	t.Helper()
	f, err := switchwire.DecodeFrame(arm.Request.Body)
	if err != nil {
		t.Fatal(err)
	}
	f.Operation, f.Entries = switchwire.OperationCheckin, nil
	f.State.Sequence++
	f.State.Deadline += 86399
	request, err := switchwire.SignFrame(f, key)
	if err != nil {
		t.Fatal(err)
	}
	e := RenewalEvidence{ConfirmedRequest: arm.Request, ConfirmedReceipts: arm.Armed, Request: request}
	for i := range keys {
		r, err := switchwire.SignFrame(switchwire.Frame{Operation: switchwire.OperationReceipt, Signer: "agent" + string(rune('0'+i)), State: f.State, Durable: f.State.Sequence, Phase: switchwire.PhaseArmed}, keys[i])
		if err != nil {
			t.Fatal(err)
		}
		e.Receipts = append(e.Receipts, r)
	}
	return e
}

func testRenewalProofRetention(t *testing.T, store *Store, draft Record, arm ArmEvidence, e RenewalEvidence) {
	t.Helper()
	partial := e
	partial.Receipts = e.Receipts[:2]
	record, err := store.ObserveRenewal(context.Background(), draft.ID, partial)
	if err != nil {
		t.Fatal(err)
	}
	status := record.RenewalStatus()
	if record.State != StateUncertain || status.Confirmed || status.Bmax-status.B0 != 86399 || status.Agents[2].State != "uncertain" {
		t.Fatalf("uncertainty lost: %+v", status)
	}
	if _, err := store.ObserveRenewal(context.Background(), draft.ID, RenewalEvidence{ConfirmedRequest: arm.Request, ConfirmedReceipts: arm.Armed, Request: e.Request}); !errors.Is(err, ErrInput) {
		t.Fatalf("receipt regression: %v", err)
	}
	record, err = store.ObserveRenewal(context.Background(), draft.ID, e)
	if err != nil || record.State != StateArmed || !record.RenewalStatus().Confirmed || record.RenewalStatus().B0 != status.Bmax {
		t.Fatalf("confirmation: %+v %v", record, err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decode(draft.ID, data); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ObserveRenewal(context.Background(), draft.ID, partial); !errors.Is(err, ErrInput) {
		t.Fatalf("confirmed regression: %v", err)
	}
	bad := e
	bad.Request.Signature = append([]byte(nil), e.Request.Signature...)
	bad.Request.Signature[0] ^= 1
	if _, err := store.ObserveRenewal(context.Background(), draft.ID, bad); !errors.Is(err, ErrInput) {
		t.Fatalf("forged owner: %v", err)
	}
}

func TestRenewalEvidenceRejectsSignedBoundaryAndMaterialChanges(t *testing.T) {
	r := Record{Version: 1, ID: "0123456789abcdef0123456789abcdef", IntervalSeconds: DefaultIntervalSeconds, GraceSeconds: DefaultGraceSeconds, CreatedAt: switchTestNow.Truncate(time.Second), State: StateArmed}
	arm, key, keys := armEvidenceKeys(t, r)
	r.Arm = &arm
	proof := signedRenewalProof(t, arm, key, keys)
	if _, _, _, err := validateRenewal(r, proof); err != nil {
		t.Fatalf("valid proof: %v", err)
	}
	cases := map[string]func(*switchwire.Frame){
		"prior deadline": func(f *switchwire.Frame) { f.State.Deadline = r.CreatedAt.Unix() + r.IntervalSeconds - 1 },
		"sequence gap":   func(f *switchwire.Frame) { f.State.Sequence = 4 },
		"content change": func(f *switchwire.Frame) { f.State.Content[0] ^= 1 },
		"key change":     func(f *switchwire.Frame) { f.State.Key[0] ^= 1 },
		"grace change":   func(f *switchwire.Frame) { f.State.Grace++ },
		"other switch":   func(f *switchwire.Frame) { f.State.Switch = "1123456789abcdef0123456789abcdef" },
		"wrong phase":    func(f *switchwire.Frame) { f.Phase = switchwire.PhasePrepared },
		"status request": func(f *switchwire.Frame) { f.Operation = switchwire.OperationStatusRequest },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			bad := proof
			f, err := switchwire.DecodeFrame(proof.Request.Body)
			if err != nil {
				t.Fatal(err)
			}
			mutate(&f)
			bad.Request, err = switchwire.SignFrame(f, key)
			if err != nil {
				t.Fatal(err)
			}
			bad.Receipts = nil
			if _, _, _, err := validateRenewal(r, bad); !errors.Is(err, ErrInput) {
				t.Fatalf("signed invalid state accepted: %v", err)
			}
		})
	}
}

func TestRenewalEvidenceRejectsUnauthenticatedConfirmation(t *testing.T) {
	r := Record{Version: 1, ID: "0123456789abcdef0123456789abcdef", IntervalSeconds: DefaultIntervalSeconds, GraceSeconds: DefaultGraceSeconds, CreatedAt: switchTestNow.Truncate(time.Second), State: StateArmed}
	arm, key, keys := armEvidenceKeys(t, r)
	r.Arm = &arm
	proof := signedRenewalProof(t, arm, key, keys)
	if _, _, _, err := validateRenewal(r, proof); err != nil {
		t.Fatalf("valid proof: %v", err)
	}
	cases := map[string]func(*Record, *RenewalEvidence){
		"arm replay":              func(_ *Record, p *RenewalEvidence) { p.Request = p.ConfirmedRequest; p.Receipts = p.ConfirmedReceipts },
		"missing arm":             func(r *Record, _ *RenewalEvidence) { r.Arm = nil },
		"unconfirmed arm":         func(r *Record, _ *RenewalEvidence) { r.Arm.Armed = r.Arm.Armed[:2] },
		"incomplete confirmation": func(_ *Record, p *RenewalEvidence) { p.ConfirmedReceipts = p.ConfirmedReceipts[:2] },
		"duplicate confirmation":  func(_ *Record, p *RenewalEvidence) { p.ConfirmedReceipts[1] = p.ConfirmedReceipts[0] },
		"forged confirmation":     func(_ *Record, p *RenewalEvidence) { p.ConfirmedRequest.Signature[0] ^= 1 },
		"forged receipt":          func(_ *Record, p *RenewalEvidence) { p.Receipts[0].Signature[0] ^= 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(struct {
				R Record
				P RenewalEvidence
			}{r, proof})
			var copy struct {
				R Record
				P RenewalEvidence
			}
			if err := json.Unmarshal(data, &copy); err != nil {
				t.Fatal(err)
			}
			mutate(&copy.R, &copy.P)
			if _, _, _, err := validateRenewal(copy.R, copy.P); !errors.Is(err, ErrInput) {
				t.Fatalf("unauthenticated evidence accepted: %v", err)
			}
			copy.R.Renewal = &copy.P
			if ValidateRecord(copy.R) == nil || copy.R.RenewalStatus() != nil {
				t.Fatal("invalid retained proof supplies status")
			}
		})
	}
}
