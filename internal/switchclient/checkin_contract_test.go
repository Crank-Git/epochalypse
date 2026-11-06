package switchclient

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestRenewalInitializationPreservesDurabilityFailures(t *testing.T) {
	for _, target := range []string{"1.request", "1.agent1.receipt"} {
		t.Run(target, func(t *testing.T) {
			p, key, r := renewalFixture(t)
			initial, err := RenewalStatus(r.path, p, key.Public().(ed25519.PublicKey))
			if err != nil || !initial.Confirmed {
				t.Fatalf("initial authority: %v", err)
			}
			cause := errors.New("synthetic initial evidence sync failure")
			restore := installPersistentSyncFailure(t, "file-sync", filepath.Join(r.path, target), cause)
			if err := InitializeRenewal(r.path, p, key, initial.Request, initial.Receipts); !errors.Is(err, cause) {
				t.Fatalf("initial evidence bypasses durability: %v", err)
			}
			restore()
			if err := InitializeRenewal(r.path, p, key, initial.Request, initial.Receipts); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRenewalRequiresConfirmedRequestDurabilityBeforeSuccessor(t *testing.T) {
	p, key, r := renewalFixture(t)
	cause := errors.New("synthetic confirmed request sync failure")
	restore := installPersistentSyncFailure(t, "file-sync", requestPath(r.path, 1), cause)
	if _, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0)); !errors.Is(err, cause) || len(r.requests) != 0 {
		t.Fatalf("unsynced request permits successor: %v", err)
	}
	restore()
	if result, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0)); err != nil || !result.Confirmed {
		t.Fatalf("durable successor: %v", err)
	}
}

func TestRenewalRefusesPublicRetainedEvidence(t *testing.T) {
	for _, name := range []string{"1.request", "1.agent1.receipt"} {
		t.Run(name, func(t *testing.T) {
			p, key, r := renewalFixture(t)
			if err := os.Chmod(filepath.Join(r.path, name), 0o644); err != nil {
				t.Fatal(err)
			} // #nosec G302 -- The fixture verifies refusal of public intent and receipts.
			if _, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0)); err == nil || len(r.requests) != 0 {
				t.Fatal("public retained evidence permits transmission")
			}
		})
	}
}

func TestRenewalInitializationRejectsIncompleteOrForeignAuthority(t *testing.T) {
	p, key, r := renewalFixture(t)
	initial, err := RenewalStatus(r.path, p, key.Public().(ed25519.PublicKey))
	if err != nil || !initial.Confirmed {
		t.Fatalf("initial authority: %v", err)
	}
	if err := InitializeRenewal(r.path, p, key, p.Prepare, initial.Receipts); err == nil {
		t.Fatal("prepare request initializes renewal")
	}
	delete(initial.Receipts, "agent1")
	if err := InitializeRenewal(r.path, p, key, initial.Request, initial.Receipts); err == nil {
		t.Fatal("incomplete receipts initialize renewal")
	}
	bad := p
	bad.Certificate = nil
	if err := InitializeRenewal(r.path, bad, key, initial.Request, initial.Receipts); err == nil {
		t.Fatal("foreign enrollment initializes renewal")
	}
	if _, err := RenewalStatus(r.path, bad, key.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("foreign enrollment reports status")
	}
	if _, err := Renew(context.Background(), r.path, p, key, nil, time.Unix(9999, 0)); err == nil {
		t.Fatal("missing transport reports renewal")
	}
}

func TestRenewalRefusesUnprotectedJournalPaths(t *testing.T) {
	for _, kind := range []string{"missing parent", "file", "public directory", "lock directory", "directory sync", "locked"} {
		t.Run(kind, func(t *testing.T) {
			p, key, r := renewalFixture(t)
			path := filepath.Join(t.TempDir(), "journal")
			switch kind {
			case "missing parent":
				path = filepath.Join(path, "absent", "journal")
			case "file":
				if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "public directory":
				if err := os.Mkdir(path, 0o755); err != nil { // #nosec G301 -- The fixture verifies rejection of a public journal directory.
					t.Fatal(err)
				}
			case "lock directory":
				if err := os.MkdirAll(filepath.Join(path, "lock"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "directory sync":
				previous := syncDirectory
				syncDirectory = func(_ string, _ *os.File) error { return errors.New("synthetic journal directory sync failure") }
				t.Cleanup(func() { syncDirectory = previous })
			case "locked":
				lock, err := lockRenewal(path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { unlockRenewal(lock) })
			}
			if _, err := RenewalStatus(path, p, key.Public().(ed25519.PublicKey)); err == nil {
				t.Fatal("unsafe journal reports status")
			}
			if err := InitializeRenewal(path, p, key, mustJournalRequest(t, r.path, 1), mustJournalReceipts(t, r, p, key)); err == nil {
				t.Fatal("unsafe journal accepts initialization")
			}
		})
	}
}

func mustJournalRequest(t *testing.T, path string, sequence uint64) switchwire.Envelope {
	t.Helper()
	r, err := readJournalEnvelope(requestPath(path, sequence))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func mustJournalReceipts(t *testing.T, r *renewalReplica, p Plan, key ed25519.PrivateKey) map[string]switchwire.Envelope {
	t.Helper()
	// The fixture journal is protected before the synthetic path failure applies.
	e, err := switchwire.AdmitCertificate(p.Certificate, p.CertificateSignature, key.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	f, err := switchwire.DecodeFrame(p.Prepare.Body)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := readRenewalReceipts(r.path, f.State, e)
	if err != nil {
		t.Fatal(err)
	}
	return receipts
}

func TestRenewalRejectsIncompleteAndConflictingRetainedAuthority(t *testing.T) {
	for _, kind := range []string{"missing initial receipt", "invalid initial signature", "wrong original intent", "wrong sequence", "invalid receipt", "unresolved successor", "changed initial deadline"} {
		t.Run(kind, func(t *testing.T) {
			p, key, r := renewalFixture(t)
			mutateRenewalJournal(t, kind, &p, key, r)
			if _, err := RenewalStatus(r.path, p, key.Public().(ed25519.PublicKey)); err == nil {
				t.Fatal("conflicting journal reports status")
			}
			if _, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0)); err == nil || len(r.requests) != 0 {
				t.Fatal("conflicting journal permits transmission")
			}
		})
	}
}

func mutateRenewalJournal(t *testing.T, kind string, p *Plan, key ed25519.PrivateKey, r *renewalReplica) {
	t.Helper()
	request := mustJournalRequest(t, r.path, 1)
	path := requestPath(r.path, 1)
	switch kind {
	case "missing initial receipt":
		if err := os.Remove(receiptPath(r.path, 1, "agent1")); err != nil {
			t.Fatal(err)
		}
		return
	case "invalid initial signature":
		request.Signature[0] ^= 1
	case "wrong original intent":
		p.Prepare = request
		return
	case "wrong sequence":
		path = requestPath(r.path, 2)
	case "invalid receipt":
		path = receiptPath(r.path, 1, "agent1")
		request = mustJournalRequest(t, r.path, 1)
	case "changed initial deadline":
		initial, err := switchwire.DecodeFrame(p.Prepare.Body)
		if err != nil {
			t.Fatal(err)
		}
		initial.State.Deadline--
		p.Prepare, err = switchwire.SignFrame(initial, key)
		if err != nil {
			t.Fatal(err)
		}
		return
	case "unresolved successor":
		request = unresolvedSuccessor(t, *p, key, r)
		path = requestPath(r.path, 3)
		r.requests = nil
	}
	data, err := switchwire.MarshalEnvelope(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func unresolvedSuccessor(t *testing.T, p Plan, key ed25519.PrivateKey, r *renewalReplica) switchwire.Envelope {
	t.Helper()
	r.lost = "agent3"
	if result, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0)); err != nil || result.Confirmed {
		t.Fatalf("uncertain fixture: %v", err)
	}
	request := mustJournalRequest(t, r.path, 2)
	frame, err := switchwire.DecodeFrame(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	frame.State.Sequence++
	request, err = switchwire.SignFrame(frame, key)
	if err != nil {
		t.Fatal(err)
	}
	return request
}
