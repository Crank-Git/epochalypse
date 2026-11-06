package service

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/agent"
	"github.com/Crank-Git/epochalypse/internal/switchclient"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestSwitchHTTPArmsAfterThreeSignedAgentReceipts(t *testing.T) {
	h, token := newSwitchHTTP(t)
	draftResponse := h.send(countdownRequest(http.MethodPost, "/api/v1/switches", `{}`, token))
	if draftResponse.Code != http.StatusCreated {
		t.Fatalf("create switch: %d %s", draftResponse.Code, draftResponse.Body)
	}
	draft := decodeSwitchRecord(t, draftResponse)
	evidence := signedSwitchArmEvidence(t, draft)
	request := func(evidence switches.ArmEvidence) *http.Request {
		body, err := json.Marshal(evidence)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "https://localhost/api/v1/switches/"+draft.ID+"/arm", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}
	partial := evidence
	partial.Armed = evidence.Armed[:2]
	uncertainResponse := h.send(request(partial))
	assertSwitchResponseState(t, uncertainResponse, http.StatusAccepted, switches.StateUncertain, 2)
	invalid := evidence
	invalid.Armed = append([]switchwire.Envelope(nil), evidence.Armed...)
	invalid.Armed[2].Signature = append([]byte(nil), invalid.Armed[2].Signature...)
	invalid.Armed[2].Signature[0] ^= 0xff
	if response := h.send(request(invalid)); response.Code != http.StatusBadRequest {
		t.Fatalf("reject invalid agent signature: %d %s", response.Code, response.Body)
	}
	status := h.send(countdownRequest(http.MethodGet, "/api/v1/switches/"+draft.ID, "", token))
	assertSwitchResponseState(t, status, http.StatusOK, switches.StateUncertain, 2)
	armedResponse := h.send(request(evidence))
	assertSwitchResponseState(t, armedResponse, http.StatusOK, switches.StateArmed, 3)
	if response := h.send(request(partial)); response.Code != http.StatusBadRequest {
		t.Fatalf("reject receipt regression: %d %s", response.Code, response.Body)
	}
	status = h.send(countdownRequest(http.MethodGet, "/api/v1/switches/"+draft.ID, "", token))
	if status.Code != http.StatusOK {
		t.Fatalf("read armed switch: %d %s", status.Code, status.Body)
	}
	retained := decodeSwitchRecord(t, status)
	assertSwitchRecordState(t, retained, switches.StateArmed, 3)
	if err := switches.ValidateRecord(retained); err != nil {
		t.Fatalf("retained signed evidence: %v", err)
	}
}

func decodeSwitchRecord(t *testing.T, response *httptest.ResponseRecorder) switches.Record {
	t.Helper()
	var record switches.Record
	if err := json.Unmarshal(response.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func assertSwitchResponseState(t *testing.T, response *httptest.ResponseRecorder, status int, state string, receipts int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("switch response: %d %s", response.Code, response.Body)
	}
	assertSwitchRecordState(t, decodeSwitchRecord(t, response), state, receipts)
}

func assertSwitchRecordState(t *testing.T, record switches.Record, state string, receipts int) {
	t.Helper()
	if record.State != state || record.Arm == nil || len(record.Arm.Armed) != receipts {
		t.Fatalf("switch state=%q receipts=%d", record.State, func() int {
			if record.Arm == nil {
				return 0
			}
			return len(record.Arm.Armed)
		}())
	}
}

func signedSwitchArmEvidence(t *testing.T, record switches.Record) switches.ArmEvidence {
	t.Helper()
	arm, _ := signedSwitchRenewalEvidence(t, record, false)
	return arm
}

func signedSwitchRenewalEvidence(t *testing.T, record switches.Record, renew bool) (switches.ArmEvidence, switches.RenewalEvidence) {
	t.Helper()
	ownerPublic, ownerPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	endpoints := newSwitchAgentEndpoints(t)
	material, err := switchcontent.Prepare(strings.NewReader("durable switch API fixture"), [3]string{endpoints.agents[0].Recipient, endpoints.agents[1].Recipient, endpoints.agents[2].Recipient})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := switchclient.NewPlan(record, material, endpoints.agents, ownerPrivate, record.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	journals := startSwitchAgentTransitions(t, record, ownerPublic, plan, endpoints)
	bundlePath := filepath.Join(t.TempDir(), "recovery.json")
	result, err := switchclient.PrepareAndArm(context.Background(), plan, ownerPrivate, &switchclient.HTTPSReplica{Pins: endpoints.pins, Roots: endpoints.roots}, bundlePath)
	if err != nil || !result.Confirmed || len(result.Armed) != 3 {
		t.Fatalf("arm through durable HTTPS agents: confirmed=%t receipts=%d err=%v", result.Confirmed, len(result.Armed), err)
	}
	// #nosec G304 -- The request file is in the test's temporary directory.
	armRequestFile, err := os.ReadFile(bundlePath + ".arm")
	if err != nil {
		t.Fatal(err)
	}
	request, err := switchwire.ReadEnvelope(strings.NewReader(string(armRequestFile)))
	if err != nil {
		t.Fatal(err)
	}
	evidence := switches.ArmEvidence{OwnerKey: [32]byte(ownerPublic), Certificate: plan.Certificate, CertificateSignature: plan.CertificateSignature, Request: request}
	for _, a := range endpoints.agents {
		receipt, ok := result.Armed[a.ID]
		if !ok {
			t.Fatalf("agent %s did not return an arm receipt", a.ID)
		}
		evidence.Armed = append(evidence.Armed, receipt)
	}
	assertSwitchArmedJournals(t, journals)
	if !renew {
		return evidence, switches.RenewalEvidence{}
	}
	path := bundlePath + ".checkins"
	if err := switchclient.InitializeRenewal(path, plan, ownerPrivate, request, result.Armed); err != nil {
		t.Fatal(err)
	}
	renewal, err := switchclient.Renew(context.Background(), path, plan, ownerPrivate, &switchclient.HTTPSReplica{Pins: endpoints.pins, Roots: endpoints.roots}, record.CreatedAt.Add(time.Second))
	if err != nil || !renewal.Confirmed {
		t.Fatalf("direct renewal fixture: %+v %v", renewal, err)
	}
	return evidence, renewal.Evidence
}

func assertSwitchArmedJournals(t *testing.T, journals [3]*agent.Journal) {
	t.Helper()
	for i, journal := range journals {
		snapshot, err := journal.Snapshot()
		if err != nil || snapshot.Phase != switchwire.PhaseArmed || len(snapshot.Receipt) == 0 {
			t.Fatalf("agent %d lacks retained arm receipt: %v", i, err)
		}
	}
}

type switchAgentEndpoints struct {
	agents     [3]switchwire.Agent
	identities [3]*age.X25519Identity
	keys       [3]ed25519.PrivateKey
	roots      *x509.CertPool
	pins       map[string][32]byte
	handlers   [3]http.Handler
}

func newSwitchAgentEndpoints(t *testing.T) *switchAgentEndpoints {
	t.Helper()
	endpoints := &switchAgentEndpoints{roots: x509.NewCertPool(), pins: make(map[string][32]byte, 3)}
	for i := range endpoints.agents {
		index := i
		id := fmt.Sprintf("agent-%c", 'a'+i)
		public, private, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		endpoints.keys[i], endpoints.identities[i] = private, identity
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { endpoints.handlers[index].ServeHTTP(w, r) }))
		t.Cleanup(server.Close)
		endpoints.roots.AddCert(server.Certificate())
		endpoints.pins[server.URL] = sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
		endpoints.agents[i] = switchwire.Agent{ID: id, Endpoint: server.URL, Recipient: identity.Recipient().String()}
		copy(endpoints.agents[i].SigningKey[:], public)
	}
	return endpoints
}

func startSwitchAgentTransitions(t *testing.T, record switches.Record, owner ed25519.PublicKey, plan switchclient.Plan, endpoints *switchAgentEndpoints) [3]*agent.Journal {
	t.Helper()
	var journals [3]*agent.Journal
	for i, enrolled := range endpoints.agents {
		root := t.TempDir()
		directory := filepath.Join(root, "authority")
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		config := agent.Config{Directory: directory, CheckpointDirectory: filepath.Join(root, "checkpoint"), ExpectedVolumeID: "test-" + enrolled.ID}
		if err := agent.Initialize(config); err != nil {
			t.Fatal(err)
		}
		journal, err := agent.Open(config)
		if err != nil {
			t.Fatal(err)
		}
		journals[i] = journal
		t.Cleanup(func() { _ = journal.Close() })
		transitions, err := agent.NewTransitions(agent.TransitionConfig{Journal: journal, OwnerKey: owner, EnrollmentBody: plan.Certificate, EnrollmentSignature: plan.CertificateSignature, AgentID: enrolled.ID, SigningKey: endpoints.keys[i], AgeIdentity: endpoints.identities[i].String(), ReadClock: switchFixtureClock(record.CreatedAt)})
		if err != nil {
			t.Fatal(err)
		}
		endpoints.handlers[i] = transitions.Handler()
	}
	return journals
}

func switchFixtureClock(issued time.Time) func() (agent.Reading, error) {
	return func() (agent.Reading, error) {
		return agent.AssessClock(agent.Assessment{Epoch: "switch-test", Implementation: "synthetic-test", Provenance: "test-only", SourceIdentities: []string{"test-a", "test-b"}, ReferenceEvidence: "synthetic", IndependenceEvidence: "synthetic", LeapPolicy: "UTC", BaseUncertainty: time.Millisecond}, agent.Capture{Epoch: "switch-test", UTCSeconds: issued.Unix() + agent.CandidateEpsilonSeconds})
	}
}

func TestSwitchBrowserEvidenceFixture(t *testing.T) {
	if os.Getenv("EPOCHALYPSE_SWITCH_BROWSER_FIXTURE") != "1" {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /fixture/switch-evidence", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "http://127.0.0.1:8080" {
			http.Error(w, "origin is not allowed", http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Set("Vary", "Origin")
		interval, err := time.ParseDuration(r.URL.Query().Get("interval_seconds") + "s")
		if err != nil {
			http.Error(w, "invalid interval", http.StatusBadRequest)
			return
		}
		grace, err := time.ParseDuration(r.URL.Query().Get("grace_seconds") + "s")
		if err != nil {
			http.Error(w, "invalid grace", http.StatusBadRequest)
			return
		}
		created, err := time.Parse(time.RFC3339, r.URL.Query().Get("created_at"))
		if err != nil {
			http.Error(w, "invalid creation time", http.StatusBadRequest)
			return
		}
		record := switches.Record{Version: 1, ID: r.URL.Query().Get("id"), IntervalSeconds: int64(interval.Seconds()), GraceSeconds: int64(grace.Seconds()), CreatedAt: created.UTC(), State: switches.StateDraft}
		evidence, renewal := signedSwitchRenewalEvidence(t, record, true)
		w.Header().Set("Content-Type", "application/json")
		response := struct {
			switches.ArmEvidence
			Renewal switches.RenewalEvidence `json:"renewal"`
		}{evidence, renewal}
		if err := json.NewEncoder(w).Encode(response); err != nil && !strings.Contains(err.Error(), "broken pipe") {
			t.Error(err)
		}
	})
	server := &http.Server{Addr: "127.0.0.1:8082", Handler: mux, ReadHeaderTimeout: time.Second}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed && err != io.EOF {
		t.Fatal(err)
	}
}
