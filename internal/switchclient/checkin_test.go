package switchclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

type renewalReplica struct {
	*replicaFixture
	path     string
	lost     string
	bad      string
	cancel   context.CancelFunc
	requests [][]byte
}

func (r *renewalReplica) Checkin(_ context.Context, a switchwire.Agent, request switchwire.Envelope) (switchwire.Envelope, error) {
	data, err := switchwire.MarshalEnvelope(request)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	f, err := switchwire.DecodeFrame(request.Body)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	stored, err := os.ReadFile(requestPath(r.path, f.State.Sequence))
	if err != nil || !bytes.Equal(data, stored) {
		r.t.Fatal("transmission precedes exact durable intent")
	}
	r.requests = append(r.requests, data)
	if a.ID == r.lost {
		return switchwire.Envelope{}, errors.New("receipt lost")
	}
	receipt, err := r.receipt(a.ID, f.State, switchwire.PhaseArmed)
	if a.ID == r.bad {
		receipt.Signature[0] ^= 1
	}
	if r.cancel != nil {
		r.cancel()
	}
	return receipt, err
}

func TestRenewalRequiresRetainedReceiptDurabilityBeforeNextSequence(t *testing.T) {
	p, key, r := renewalFixture(t)
	cause := errors.New("synthetic retained receipt sync failure")
	recoverSync := installPersistentSyncFailure(t, "file-sync", receiptPath(r.path, 2, "agent3"), cause)
	_, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0))
	if !errors.Is(err, cause) || len(r.requests) != 3 {
		t.Fatalf("receipt failure: %v", err)
	}
	r.requests = nil
	for range 2 {
		_, err = Renew(context.Background(), r.path, p, key, r, time.Unix(20000, 0))
		if !errors.Is(err, cause) || len(r.requests) != 0 {
			t.Fatalf("unsynced receipt authorizes next sequence: sends=%d error=%v", len(r.requests), err)
		}
	}
	if _, err := os.Stat(requestPath(r.path, 3)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("next intent exists before receipt durability: %v", err)
	}
	recoverSync()
	result, err := Renew(context.Background(), r.path, p, key, r, time.Unix(20000, 0))
	if err != nil || !result.Confirmed || len(r.requests) != 3 {
		t.Fatalf("durable restart: %+v %v", result, err)
	}
}

func TestRenewalJournalRetriesExactBytesAfterRestart(t *testing.T) {
	p, key, r := renewalFixture(t)
	path := r.path
	r.lost = "agent3"
	first, err := Renew(context.Background(), path, p, key, r, time.Unix(9999, 0))
	if err != nil {
		t.Fatal(err)
	}
	if first.Confirmed || first.B0 != 10060 || first.Bmax != 13659 || len(first.Receipts) != 2 {
		t.Fatalf("lost receipt hides uncertainty: %+v", first)
	}
	if first.Agents[2].State != "uncertain" {
		t.Fatalf("missing evidence: %+v", first.Agents)
	}
	r.lost = ""
	second, err := Renew(context.Background(), path, p, key, r, time.Unix(20000, 0))
	if err != nil || !second.Confirmed || second.B0 != first.Bmax {
		t.Fatalf("restart retry: %+v %v", second, err)
	}
	for _, sent := range r.requests {
		if !bytes.Equal(sent, r.requests[0]) {
			t.Fatal("restart changes signed intent")
		}
	}
	status, err := RenewalStatus(path, p, key.Public().(ed25519.PublicKey))
	if err != nil || !status.Confirmed {
		t.Fatalf("retained status: %+v %v", status, err)
	}
}

func renewalFixture(t *testing.T) (Plan, ed25519.PrivateKey, *renewalReplica) {
	t.Helper()
	p, key, replica := armFixture(t)
	return p, key, seedRenewalFixture(t, p, key, replica)
}

func seedRenewalFixture(t *testing.T, p Plan, key ed25519.PrivateKey, replica *replicaFixture) *renewalReplica {
	t.Helper()
	arm, err := PrepareAndArm(context.Background(), p, key, replica, replica.bundle)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(replica.bundle + ".arm")
	if err != nil {
		t.Fatal(err)
	}
	request, err := switchwire.ReadEnvelope(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "journal")
	if err := InitializeRenewal(path, p, key, request, arm.Armed); err != nil {
		t.Fatal(err)
	}
	return &renewalReplica{replicaFixture: replica, path: path}
}

func TestRenewalDefaultUncertaintyApproaches24Hours(t *testing.T) {
	p, key, replica := armFixture(t)
	e, err := switchwire.AdmitCertificate(p.Certificate, p.CertificateSignature, key.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	f, err := switchwire.DecodeFrame(p.Prepare.Body)
	if err != nil {
		t.Fatal(err)
	}
	material := switchcontent.Material{Ciphertext: p.Ciphertext, ContentDigest: f.State.Content, KeyID: f.State.Key, Shares: [3][]byte{f.Entries[0].Data, f.Entries[1].Data, f.Entries[2].Data}}
	draft := switches.Record{Version: 1, ID: "0123456789abcdef0123456789abcdef", State: switches.StateDraft, CreatedAt: time.Unix(1000, 0).UTC(), IntervalSeconds: switches.DefaultIntervalSeconds, GraceSeconds: switches.DefaultGraceSeconds}
	p, err = NewPlan(draft, material, e.Agents, key, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	r := seedRenewalFixture(t, p, key, replica)
	r.lost = "agent3"
	result, err := Renew(context.Background(), r.path, p, key, r, time.Unix(1000+switches.DefaultIntervalSeconds-1, 0))
	if err != nil || result.Confirmed || result.Bmax-result.B0 != 86399 || result.Grace != 3600 {
		t.Fatalf("default uncertainty: %+v %v", result, err)
	}
}

func TestRenewalTimeoutPreservesEachReceivedReceipt(t *testing.T) {
	p, key, r := renewalFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.cancel = cancel
	result, err := Renew(ctx, r.path, p, key, r, time.Unix(9999, 0))
	if !errors.Is(err, context.Canceled) || result.Confirmed || result.B0 != 10060 || result.Bmax != 13659 || result.Agents[0].State != "durable" || result.Agents[1].State != "uncertain" {
		t.Fatalf("timeout loses partial evidence: %+v %v", result, err)
	}
}

func TestRenewalRejectsAlteredAgentReceipt(t *testing.T) {
	p, key, r := renewalFixture(t)
	r.bad = "agent2"
	result, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0))
	if err != nil || result.Confirmed || len(result.Receipts) != 2 || result.Agents[1].State != "uncertain" {
		t.Fatalf("altered receipt counts as durable: %+v %v", result, err)
	}
}

func TestRenewalRejectsCorruptRetainedEvidence(t *testing.T) {
	for _, filename := range []string{"1.request", "1.agent1.receipt"} {
		t.Run(filename, func(t *testing.T) {
			p, key, r := renewalFixture(t)
			if err := os.WriteFile(filepath.Join(r.path, filename), []byte("corrupt"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := RenewalStatus(r.path, p, key.Public().(ed25519.PublicKey)); err == nil {
				t.Fatal("corrupt evidence reports status")
			}
			if _, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0)); err == nil || len(r.requests) != 0 {
				t.Fatal("corrupt evidence permits transmission")
			}
		})
	}
}

func TestRenewalRequiresIntentDurabilityBeforeEverySend(t *testing.T) {
	for _, operation := range []string{"file-sync", "directory-sync"} {
		t.Run(operation, func(t *testing.T) {
			p, key, r := renewalFixture(t)
			cause := errors.New("synthetic intent sync failure")
			recoverSync := installPersistentSyncFailure(t, operation, requestPath(r.path, 2), cause)
			for range 2 {
				_, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0))
				if !errors.Is(err, cause) || len(r.requests) != 0 {
					t.Fatalf("send bypasses durable intent: %v", err)
				}
			}
			before, err := os.ReadFile(requestPath(r.path, 2))
			if err != nil {
				t.Fatal(err)
			}
			recoverSync()
			result, err := Renew(context.Background(), r.path, p, key, r, time.Unix(20000, 0))
			if err != nil || !result.Confirmed || !bytes.Equal(r.requests[0], before) {
				t.Fatalf("durable retry changes intent: %v", err)
			}
		})
	}
}

func TestRenewalReportsEveryUnavailableAgentIndependently(t *testing.T) {
	for _, missing := range []string{"agent1", "agent2", "agent3"} {
		t.Run(missing, func(t *testing.T) {
			p, key, r := renewalFixture(t)
			r.lost = missing
			result, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0))
			if err != nil || result.Confirmed || len(result.Receipts) != 2 || len(result.Agents) != 3 {
				t.Fatalf("partial result: %+v %v", result, err)
			}
			for _, a := range result.Agents {
				if (a.State == "uncertain") != (a.ID == missing) {
					t.Fatalf("agent evidence: %+v", a)
				}
			}
		})
	}
}

func TestRenewalRejectsConcurrentJournalAccess(t *testing.T) {
	p, key, r := renewalFixture(t)
	lock, err := lockRenewal(r.path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlockRenewal(lock)
	if _, err := Renew(context.Background(), r.path, p, key, r, time.Unix(9999, 0)); err == nil || len(r.requests) != 0 {
		t.Fatal("concurrent renewal sends a competing sequence")
	}
}

func TestRenewalRejectsInvalidTimeAndOwnerBeforeTransmission(t *testing.T) {
	for _, issued := range []time.Time{{}, time.Unix(-1, 0), time.Unix(100, 0), time.Unix(1<<62+1, 0)} {
		p, key, r := renewalFixture(t)
		if _, err := Renew(context.Background(), r.path, p, key, r, issued); err == nil || len(r.requests) != 0 {
			t.Fatal("invalid issue time transmits")
		}
	}
	p, _, r := renewalFixture(t)
	_, other, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Renew(context.Background(), r.path, p, other, r, time.Unix(9999, 0)); err == nil || len(r.requests) != 0 {
		t.Fatal("another owner transmits")
	}
}
