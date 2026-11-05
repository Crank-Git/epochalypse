package switchclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
)

func TestMaximumContentSurvivesHTTPSJournalAndBundle(t *testing.T) {
	owner, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	var handlers [3]http.Handler
	agents, identities, keys, transport := localEndpoints(t, &handlers)
	plaintext := bytes.Repeat([]byte("x"), 10<<20)
	material, err := switchcontent.Prepare(bytes.NewReader(plaintext), [3]string{agents[0].Recipient, agents[1].Recipient, agents[2].Recipient})
	if err != nil {
		t.Fatal(err)
	}
	draft := switches.Record{Version: 1, ID: "0123456789abcdef0123456789abcdef", IntervalSeconds: 3600, GraceSeconds: 60, CreatedAt: time.Unix(1000, 0).UTC(), State: switches.StateDraft}
	p, err := NewPlan(draft, material, agents, key, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	journals, configs := localJournals(t, p, owner, agents, keys, identities, &handlers)
	bundlePath := filepath.Join(t.TempDir(), "recovery.json")
	result, err := PrepareAndArm(context.Background(), p, key, transport, bundlePath)
	if err != nil || !result.Confirmed {
		t.Fatalf("maximum content arm: confirmed=%t error=%v", result.Confirmed, err)
	}
	if _, err := readBundleFile(bundlePath, owner); err != nil {
		t.Fatalf("read maximum content bundle: %v", err)
	}
	shares := restartLocalJournals(t, journals, configs, identities, material.Ciphertext)
	recovered, err := switchcontent.Recover(material.Ciphertext, shares[:2])
	if err != nil || !bytes.Equal(recovered, plaintext) {
		t.Fatalf("recover maximum content: bytes=%d error=%v", len(recovered), err)
	}
}

func TestReadBundleRejectsInputOverLimit(t *testing.T) {
	if _, err := ReadBundle(bytes.NewReader(make([]byte, maxBundleBytes+1)), nil); err == nil {
		t.Fatal("oversized bundle was accepted")
	}
}

func TestArmRetryRejectsCorruptPrivateBundle(t *testing.T) {
	p, key, replicas := armFixture(t)
	result, err := PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle)
	if err != nil || !result.Confirmed {
		t.Fatalf("initial arm: confirmed=%t error=%v", result.Confirmed, err)
	}
	if err := os.Chmod(replicas.bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replicas.bundle, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle)
	if err == nil || result.Confirmed || replicas.armCalls != 3 {
		t.Fatalf("corrupt bundle retry: confirmed=%t calls=%d error=%v", result.Confirmed, replicas.armCalls, err)
	}
}

func TestArmRetainsMissingResponseAndAcceptsExactRetry(t *testing.T) {
	p, key, replicas := armFixture(t)
	replicas.failArm = "agent3"
	result, err := PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle)
	if err == nil || result.Confirmed || len(result.Armed) != 2 || result.Errors["agent3"] != "agent response unavailable" {
		t.Fatalf("missing response: result=%+v error=%v", result, err)
	}
	owner := key.Public().(ed25519.PublicKey)
	if _, err := readBundleFile(replicas.bundle, owner); err != nil {
		t.Fatalf("read retained recovery bundle: %v", err)
	}
	replicas.failArm = ""
	replicas.failPrepare = "agent1"
	result, err = PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle)
	if err != nil || !result.Confirmed || replicas.armCalls != 6 {
		t.Fatalf("exact retry: result=%+v calls=%d error=%v", result, replicas.armCalls, err)
	}
}

func readBundleFile(path string, owner ed25519.PublicKey) (Bundle, error) {
	file, err := os.Open(path) // #nosec G304 -- The test creates this file in its temporary directory.
	if err != nil {
		return Bundle{}, err
	}
	defer func() { _ = file.Close() }()
	return ReadBundle(file, owner)
}
