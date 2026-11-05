package switches

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func armEvidence(t *testing.T, record Record) ArmEvidence {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := switchwire.Certificate{Version: switchwire.Version, Interval: uint64(record.IntervalSeconds), Grace: uint64(record.GraceSeconds), Key: [32]byte{1}, Content: [32]byte{2}} // #nosec G115 -- The fixture uses bounded draft durations.
	copy(c.OwnerKey[:], pub)
	keys := [3]ed25519.PrivateKey{}
	for i := range c.Agents {
		public, private, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		a := switchwire.Agent{ID: fmt.Sprintf("agent%d", i), Endpoint: fmt.Sprintf("https://agent%d.invalid", i), Recipient: identity.Recipient().String()}
		copy(a.SigningKey[:], public)
		c.Agents[i], keys[i] = a, private
	}
	e := ArmEvidence{OwnerKey: c.OwnerKey}
	e.Certificate, err = switchwire.EncodeCertificate(c)
	if err != nil {
		t.Fatal(err)
	}
	e.CertificateSignature = ed25519.Sign(key, e.Certificate)
	s := switchwire.State{Switch: record.ID, Version: switchwire.Version, Sequence: 1, Deadline: record.CreatedAt.Unix() + record.IntervalSeconds, Grace: c.Grace, Content: c.Content, Key: c.Key, Enrollment: sha256.Sum256(e.Certificate)}
	f := switchwire.Frame{Operation: switchwire.OperationArm, Signer: "owner", State: s, Phase: switchwire.PhaseArmed}
	for i, a := range c.Agents {
		prepared, err := switchwire.SignFrame(switchwire.Frame{Operation: switchwire.OperationReceipt, Signer: a.ID, State: s, Durable: 1, Phase: switchwire.PhasePrepared}, keys[i])
		if err != nil {
			t.Fatal(err)
		}
		data, err := switchwire.MarshalEnvelope(prepared)
		if err != nil {
			t.Fatal(err)
		}
		f.Entries = append(f.Entries, switchwire.Entry{ID: a.ID, Key: a.SigningKey, Data: data})
		armed, err := switchwire.SignFrame(switchwire.Frame{Operation: switchwire.OperationReceipt, Signer: a.ID, State: s, Durable: 1, Phase: switchwire.PhaseArmed}, keys[i])
		if err != nil {
			t.Fatal(err)
		}
		e.Armed = append(e.Armed, armed)
	}
	e.Request, err = switchwire.SignFrame(f, key)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestObserveArmRequiresThreeReceiptsAndPreservesEvidenceAfterRestart(t *testing.T) {
	dir := t.TempDir()
	shared, store := newDraftStore(t, dir)
	draft, err := store.Create(context.Background(), Input{}, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	evidence := armEvidence(t, draft)
	partial := evidence
	partial.Armed = evidence.Armed[:1]
	record, err := store.ObserveArm(context.Background(), draft.ID, partial)
	if err != nil || record.State != "uncertain" {
		t.Fatalf("partial evidence state=%s: %v", record.State, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}
	_, store = newDraftStore(t, dir)
	record, err = store.ObserveArm(context.Background(), draft.ID, evidence)
	if err != nil || record.State != "armed" {
		t.Fatalf("complete evidence state=%s: %v", record.State, err)
	}
	retained, err := store.Get(draft.ID)
	if err != nil || retained.State != "armed" {
		t.Fatalf("retained state=%s: %v", retained.State, err)
	}
	data, err := json.Marshal(retained)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decode(draft.ID, data); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ObserveArm(context.Background(), draft.ID, partial); !errors.Is(err, ErrInput) {
		t.Fatalf("stale observation replaces confirmed evidence: %v", err)
	}
}

func TestObserveArmPreservesBindingAndChargesSharedQuota(t *testing.T) {
	shared, store := newDraftStore(t, t.TempDir())
	draft, err := store.Create(context.Background(), Input{}, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	evidence := armEvidence(t, draft)
	shared.Limits.MaxBytes = 4096
	if _, err := store.ObserveArm(context.Background(), draft.ID, evidence); !errors.Is(err, installation.ErrQuota) {
		t.Fatalf("arm evidence bypasses byte quota: %v", err)
	}
	got, err := store.Get(draft.ID)
	if err != nil || got != draft {
		t.Fatalf("quota failure changes draft: %v", err)
	}
	shared.Limits = installation.DefaultLimits()
	if _, err := store.ObserveArm(context.Background(), draft.ID, evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ObserveArm(context.Background(), draft.ID, armEvidence(t, draft)); !errors.Is(err, ErrInput) {
		t.Fatalf("a new enrollment replaces retained evidence: %v", err)
	}
}

func TestArmEvidenceRejectsInvalidBindingsAndReceipts(t *testing.T) {
	_, store := newDraftStore(t, t.TempDir())
	draft, err := store.Create(context.Background(), Input{}, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	evidence := armEvidence(t, draft)
	for _, name := range []string{"owner", "certificate", "request", "signature", "duplicate", "excess", "interval", "grace", "id", "version", "time"} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(evidence)
			if err != nil {
				t.Fatal(err)
			}
			var invalid ArmEvidence
			if err := json.Unmarshal(data, &invalid); err != nil {
				t.Fatal(err)
			}
			record := draft
			mutateArmEvidence(name, &record, &invalid)
			if err := ValidateArmEvidence(record, invalid); !errors.Is(err, ErrInput) {
				t.Fatalf("invalid evidence: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ObserveArm(ctx, draft.ID, evidence); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := store.ObserveArm(context.Background(), "bad", evidence); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
	if _, err := store.ObserveArm(context.Background(), "0123456789abcdef0123456789abcdef", evidence); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func mutateArmEvidence(name string, record *Record, invalid *ArmEvidence) {
	switch name {
	case "owner":
		invalid.OwnerKey[0] ^= 1
	case "certificate":
		invalid.CertificateSignature[0] ^= 1
	case "request":
		invalid.Request.Signature[0] ^= 1
	case "signature":
		invalid.Armed[0].Signature[0] ^= 1
	case "duplicate":
		invalid.Armed[1] = invalid.Armed[0]
	case "excess":
		invalid.Armed = append(invalid.Armed, invalid.Armed[0])
	case "interval":
		record.IntervalSeconds++
	case "grace":
		record.GraceSeconds++
	case "id":
		record.ID = "0123456789abcdef0123456789abcdef"
	case "version":
		record.Version++
	case "time":
		record.CreatedAt = record.CreatedAt.AddDate(0, 0, 1)
	}
}
