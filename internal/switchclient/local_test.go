package switchclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/agent"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestLocalHTTPSArmPersistsThreeReplicasAndRecoveryBundle(t *testing.T) {
	owner, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	var handlers [3]http.Handler
	agents, identities, keys, transport := localEndpoints(t, &handlers)
	material, err := switchcontent.Prepare(bytes.NewBufferString("local authenticated switch content"), [3]string{agents[0].Recipient, agents[1].Recipient, agents[2].Recipient})
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
		t.Fatalf("local arm: %+v %v", result, err)
	}
	result, err = PrepareAndArm(context.Background(), p, key, transport, bundlePath)
	if err != nil || !result.Confirmed {
		t.Fatalf("exact local retry: %+v %v", result, err)
	}
	shares := restartLocalJournals(t, journals, configs, identities, material.Ciphertext)
	plaintext, err := switchcontent.Recover(material.Ciphertext, shares[:2])
	if err != nil || string(plaintext) != "local authenticated switch content" {
		t.Fatalf("recovery: %s %v", plaintext, err)
	}
	file, err := os.Open(bundlePath) // #nosec G304 -- The test creates this synthetic bundle in its temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if _, err := ReadBundle(file, owner); err != nil {
		t.Fatal(err)
	}
}

func labReading() (agent.Reading, error) {
	return agent.AssessClock(agent.Assessment{Epoch: "explicit-test", Implementation: "synthetic-test", Provenance: "test-only", SourceIdentities: []string{"test-a", "test-b"}, ReferenceEvidence: "synthetic", IndependenceEvidence: "synthetic", LeapPolicy: "UTC", BaseUncertainty: time.Millisecond}, agent.Capture{Epoch: "explicit-test", UTCSeconds: 1000})
}

func localEndpoints(t *testing.T, handlers *[3]http.Handler) ([3]switchwire.Agent, [3]*age.X25519Identity, [3]ed25519.PrivateKey, *HTTPSReplica) {
	t.Helper()
	var agents [3]switchwire.Agent
	var identities [3]*age.X25519Identity
	var keys [3]ed25519.PrivateKey
	roots := x509.NewCertPool()
	transport := &HTTPSReplica{Pins: map[string][32]byte{}, Roots: roots}
	for i, id := range []string{"agent-a", "agent-b", "agent-c"} {
		pub, private, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = private
		identities[i], err = age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handlers[i].ServeHTTP(w, r) }))
		t.Cleanup(server.Close)
		roots.AddCert(server.Certificate())
		transport.Pins[server.URL] = sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
		agents[i] = switchwire.Agent{ID: id, Recipient: identities[i].Recipient().String(), Endpoint: server.URL}
		copy(agents[i].SigningKey[:], pub)
	}
	return agents, identities, keys, transport
}

func localJournals(t *testing.T, p Plan, owner ed25519.PublicKey, agents [3]switchwire.Agent, keys [3]ed25519.PrivateKey, identities [3]*age.X25519Identity, handlers *[3]http.Handler) ([3]*agent.Journal, [3]agent.Config) {
	t.Helper()
	var journals [3]*agent.Journal
	var configs [3]agent.Config
	for i, a := range agents {
		root := t.TempDir()
		directory := filepath.Join(root, "authority")
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		configs[i] = agent.Config{Directory: directory, CheckpointDirectory: filepath.Join(root, "checkpoint"), ExpectedVolumeID: "lab-" + a.ID}
		if err := agent.Initialize(configs[i]); err != nil {
			t.Fatal(err)
		}
		journal, err := agent.Open(configs[i])
		journals[i] = journal
		if err != nil {
			t.Fatal(err)
		}
		transitions, err := agent.NewTransitions(agent.TransitionConfig{Journal: journals[i], OwnerKey: owner, EnrollmentBody: p.Certificate, EnrollmentSignature: p.CertificateSignature, AgentID: a.ID, SigningKey: keys[i], AgeIdentity: identities[i].String(), ReadClock: labReading})
		if err != nil {
			t.Fatal(err)
		}
		handlers[i] = transitions.Handler()
	}
	return journals, configs
}

func restartLocalJournals(t *testing.T, journals [3]*agent.Journal, configs [3]agent.Config, identities [3]*age.X25519Identity, ciphertext []byte) [][]byte {
	t.Helper()
	var shares [][]byte
	for i, journal := range journals {
		if err := journal.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := agent.Open(configs[i])
		if err != nil {
			t.Fatal(err)
		}
		state, err := reopened.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if state.Phase != switchwire.PhaseArmed || !bytes.Equal(state.Ciphertext, ciphertext) || len(state.Receipt) == 0 {
			t.Fatal("restart loses armed material")
		}
		share, err := switchcontent.DecryptShare(state.EncryptedShare, identities[i])
		if err != nil {
			t.Fatal(err)
		}
		shares = append(shares, share)
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return shares
}
