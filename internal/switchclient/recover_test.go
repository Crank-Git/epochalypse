package switchclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/agent"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestRecoverUsesTwoPublicAgentsWithoutMainService(t *testing.T) {
	owner, bundle, transport, _, _ := publicRecoveryFixture(t)
	plaintext, err := Recover(context.Background(), bundle, owner, transport)
	if err != nil || string(plaintext) != "independent public recovery" {
		t.Fatalf("recover: %q %v", plaintext, err)
	}
}

func publicRecoveryFixture(t *testing.T) (ed25519.PublicKey, Bundle, *HTTPSReplica, *[3]http.Handler, [3]ed25519.PrivateKey) {
	t.Helper()
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
	file, err := os.Open(path) // #nosec G304 -- The test reads its own temporary recovery bundle.
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
	handlers[2] = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "offline", http.StatusServiceUnavailable) })
	return owner, bundle, transport, &handlers, keys
}

func TestRecoverWithholdsPlaintextForInvalidPublicMaterial(t *testing.T) {
	for _, fault := range []string{"manifest", "certificate", "signature", "accepted", "share", "duplicate", "ciphertext", "offline"} {
		t.Run(fault, func(t *testing.T) {
			owner, bundle, transport, handlers, _ := publicRecoveryFixture(t)
			first := handlers[0]
			handlers[0] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recorded := httptest.NewRecorder()
				first.ServeHTTP(recorded, r)
				data := recorded.Body.Bytes()
				if r.URL.Path == "/agent/v1/public/switches/"+bundle.Plan.State.Switch+"/ciphertext" {
					if fault == "ciphertext" {
						data = []byte("corrupt ciphertext")
					}
				} else {
					data = invalidPublicMaterial(t, fault, data)
				}
				if fault == "offline" {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
				_, _ = w.Write(data)
			})
			if fault == "duplicate" {
				handlers[0] = handlers[1]
			}
			plaintext, err := Recover(context.Background(), bundle, owner, transport)
			if err == nil || len(plaintext) != 0 {
				t.Fatalf("invalid %s exposes plaintext: %q %v", fault, plaintext, err)
			}
		})
	}
}

func invalidPublicMaterial(t *testing.T, fault string, data []byte) []byte {
	t.Helper()
	if fault == "manifest" {
		return []byte(`{"unknown":true}`)
	}
	var manifest switchwire.PublicManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Error(err)
		return nil
	}
	switch fault {
	case "certificate":
		manifest.Certificate[0] ^= 1
	case "signature":
		manifest.Release.Signature[0] ^= 1
	case "accepted":
		manifest.Accepted.Signature[0] ^= 1
	case "share":
		manifest.Release.Body[len(manifest.Release.Body)-1] ^= 1
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Error(err)
		return nil
	}
	return encoded
}

func TestRecoverRejectsAuthenticatedWrongAndDuplicateShares(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		owner, bundle, transport, handlers, keys := publicRecoveryFixture(t)
		request := httptest.NewRequest(http.MethodGet, "/agent/v1/public/switches/"+bundle.Plan.State.Switch, nil)
		first, second := httptest.NewRecorder(), httptest.NewRecorder()
		handlers[0].ServeHTTP(first, request)
		handlers[1].ServeHTTP(second, request)
		var manifest, other switchwire.PublicManifest
		if err := json.Unmarshal(first.Body.Bytes(), &manifest); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(second.Body.Bytes(), &other); err != nil {
			t.Fatal(err)
		}
		frame, err := switchwire.DecodeFrame(manifest.Release.Body)
		if err != nil {
			t.Fatal(err)
		}
		if duplicate {
			otherFrame, err := switchwire.DecodeFrame(other.Release.Body)
			if err != nil {
				t.Fatal(err)
			}
			frame.Entries[0].Data = otherFrame.Entries[0].Data
		} else {
			frame.Entries[0].Data[1] ^= 1
		}
		signed, err := switchwire.SignFrame(frame, keys[0])
		if err != nil {
			t.Fatal(err)
		}
		manifest.Release = &signed
		encoded, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		original := handlers[0]
		handlers[0] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == request.URL.Path {
				_, _ = w.Write(encoded)
				return
			}
			original.ServeHTTP(w, r)
		})
		plaintext, err := Recover(context.Background(), bundle, owner, transport)
		if err == nil || len(plaintext) != 0 {
			t.Fatalf("authenticated invalid share exposes plaintext: duplicate=%v %q %v", duplicate, plaintext, err)
		}
	}
}
