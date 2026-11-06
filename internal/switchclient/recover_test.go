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

	"github.com/Crank-Git/epochalypse/internal/agent"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
)

func TestRecoverUsesTwoPublicAgentsWithoutMainService(t *testing.T) {
	owner, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	var handlers [3]http.Handler
	agents, identities, keys, transport := localEndpoints(t, &handlers)
	material, err := switchcontent.Prepare(bytes.NewBufferString("independent public recovery"), [3]string{agents[0].Recipient, agents[1].Recipient, agents[2].Recipient})
	if err != nil {
		t.Fatal(err)
	}
	draft := switches.Record{Version: 1, ID: "0123456789abcdef0123456789abcdef", IntervalSeconds: 3600, GraceSeconds: 60, CreatedAt: time.Unix(1000, 0).UTC(), State: switches.StateDraft}
	p, err := NewPlan(draft, material, agents, key, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	journals, _ := localJournals(t, p, owner, agents, keys, identities, &handlers)
	for _, journal := range journals {
		t.Cleanup(func() { _ = journal.Close() })
	}
	path := filepath.Join(t.TempDir(), "bundle.json")
	result, err := PrepareAndArm(context.Background(), p, key, transport, path)
	if err != nil || !result.Confirmed {
		t.Fatalf("arm: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := ReadBundle(file, owner)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		tr, err := agent.NewTransitions(agent.TransitionConfig{Journal: journals[i], OwnerKey: owner, EnrollmentBody: p.Certificate, EnrollmentSignature: p.CertificateSignature, AgentID: agents[i].ID, SigningKey: keys[i], AgeIdentity: identities[i].String(), ReadClock: func() (agent.Reading, error) {
			return agent.AssessClock(agent.Assessment{Epoch: "test", Implementation: "synthetic", Provenance: "test", SourceIdentities: []string{"a", "b"}, ReferenceEvidence: "test", IndependenceEvidence: "test", LeapPolicy: "UTC"}, agent.Capture{Epoch: "test", UTCSeconds: 4662})
		}})
		if err != nil {
			t.Fatal(err)
		}
		handlers[i] = tr.Handler()
	}
	handlers[2] = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "offline", 503) })
	plaintext, err := Recover(context.Background(), bundle, owner, transport)
	if err != nil || string(plaintext) != "independent public recovery" {
		t.Fatalf("recover: %q %v", plaintext, err)
	}
}
