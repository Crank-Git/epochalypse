package agent

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestTransitionsPrepareStoresOwnShareAndReturnsSameReceipt(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := fixture.material.Ciphertext
	request := fixture.prepareRequest(t, ciphertext)
	first, err := transitions.Prepare(request, ciphertext, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := transitions.Prepare(request, ciphertext, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Body, second.Body) || !bytes.Equal(first.Signature, second.Signature) {
		t.Fatal("exact retry changed the durable receipt")
	}
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != uint8(switchwire.PhasePrepared) || len(state.EncryptedShare) == 0 || !bytes.Equal(state.Ciphertext, ciphertext) {
		t.Fatal("prepare did not persist the phase, ciphertext, and encrypted share")
	}
}

func TestTransitionsPrepareRetryReturnsReceiptAfterJournalRestart(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	receipt, err := transitions.Prepare(request, fixture.material.Ciphertext, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.journal, err = Open(fixture.authorityConfig)
	if err != nil {
		t.Fatal(err)
	}
	transitions, err = NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	retried, err := transitions.Prepare(request, fixture.material.Ciphertext, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(receipt.Body, retried.Body) || !bytes.Equal(receipt.Signature, retried.Signature) {
		t.Fatal("journal restart changed the durable receipt")
	}
}

func TestTransitionsPrepareRejectsWrongCiphertextBeforeClock(t *testing.T) {
	fixture := newTransitionFixture(t)
	clockCalls := 0
	config := fixture.config(0)
	config.ReadClock = func() (Reading, error) {
		clockCalls++
		return Reading{lower: 1000}, nil
	}
	transitions, err := NewTransitions(config)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := bytes.Clone(fixture.material.Ciphertext)
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err := transitions.Prepare(fixture.prepareRequest(t, fixture.material.Ciphertext), ciphertext, nil); err == nil {
		t.Fatal("prepare accepted ciphertext with a different digest")
	}
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if state.Generation != 0 || clockCalls != 0 {
		t.Fatalf("invalid content reached clock or journal: generation=%d clockCalls=%d", state.Generation, clockCalls)
	}
}

func TestTransitionsRejectsWrongLocalSigningKey(t *testing.T) {
	fixture := newTransitionFixture(t)
	config := fixture.config(0)
	_, config.SigningKey, _ = ed25519.GenerateKey(rand.Reader)
	if _, err := NewTransitions(config); err == nil {
		t.Fatal("constructor accepted a signing key outside enrollment")
	}
}

func TestTransitionsRejectsCertificateWithChangedSignature(t *testing.T) {
	fixture := newTransitionFixture(t)
	config := fixture.config(0)
	config.EnrollmentSignature = bytes.Clone(config.EnrollmentSignature)
	config.EnrollmentSignature[0] ^= 1
	if _, err := NewTransitions(config); err == nil {
		t.Fatal("constructor accepted a modified enrollment signature")
	}
}

func TestTransitionsRejectsFutureIssueTimeBeforePersistence(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequestAt(t, fixture.material.Ciphertext, 8000)
	if _, err := transitions.Prepare(request, fixture.material.Ciphertext, nil); err == nil {
		t.Fatal("prepare accepted an issue time beyond the clock tolerance")
	}
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if state.Generation != 0 {
		t.Fatal("future request changed durable state")
	}
}

func TestTransitionsRejectsOwnerSignatureBeforeClock(t *testing.T) {
	fixture := newTransitionFixture(t)
	clockCalls := 0
	config := fixture.config(0)
	config.ReadClock = func() (Reading, error) {
		clockCalls++
		return Reading{lower: 1000}, nil
	}
	transitions, err := NewTransitions(config)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	request.Signature = bytes.Clone(request.Signature)
	request.Signature[0] ^= 1
	if _, err := transitions.Prepare(request, fixture.material.Ciphertext, nil); err == nil {
		t.Fatal("prepare accepted an invalid owner signature")
	}
	if clockCalls != 0 {
		t.Fatal("invalid owner request reached the clock")
	}
}

func TestTransitionsPossibleDisclosureLatchBlocksPrepare(t *testing.T) {
	fixture := newTransitionFixture(t)
	if _, err := fixture.journal.Commit(State{Generation: 1, PossibleDisclosure: true}); err != nil {
		t.Fatal(err)
	}
	clockCalls := 0
	config := fixture.config(0)
	config.ReadClock = func() (Reading, error) {
		clockCalls++
		return Reading{lower: 1000}, nil
	}
	transitions, err := NewTransitions(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transitions.Prepare(fixture.prepareRequest(t, fixture.material.Ciphertext), fixture.material.Ciphertext, nil); !errors.Is(err, ErrClock) {
		t.Fatalf("prepare after disclosure error = %v", err)
	}
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !state.PossibleDisclosure || clockCalls != 0 {
		t.Fatal("prepare ignored the irreversible disclosure latch")
	}
}

func TestTransitionsClockFailurePersistsUnsafeLatch(t *testing.T) {
	fixture := newTransitionFixture(t)
	config := fixture.config(0)
	config.ReadClock = func() (Reading, error) { return Reading{}, errors.New("clock unavailable") }
	transitions, err := NewTransitions(config)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	if _, err := transitions.Prepare(request, fixture.material.Ciphertext, nil); !errors.Is(err, ErrClock) {
		t.Fatalf("clock failure error = %v", err)
	}
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !state.UnsafeClock || state.Phase != 0 {
		t.Fatal("clock failure did not persist quarantine before response")
	}
}

func TestTransitionsHandlerRejectsWrongSwitchPath(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/agent/v1/switches/other/prepare", strings.NewReader(`{"request":{"Body":"","Signature":""}}`))
	transitions.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("wrong switch path status = %d", recorder.Code)
	}
}

func TestTransitionsArmRequiresThreeMatchingPreparedReceipts(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := fixture.material.Ciphertext
	preparedRequest := fixture.prepareRequest(t, ciphertext)
	if _, err := transitions.Prepare(preparedRequest, ciphertext, nil); err != nil {
		t.Fatal(err)
	}
	prepared, err := switchwire.VerifyOwnerRequest(preparedRequest, fixture.enrollment)
	if err != nil {
		t.Fatal(err)
	}
	receipts := make([]switchwire.Envelope, 0, 3)
	for index, agent := range fixture.certificate.Agents {
		receipt, err := switchwire.SignFrame(switchwire.Frame{
			Operation: switchwire.OperationReceipt, Signer: agent.ID, State: prepared.State,
			Durable: prepared.State.Sequence, Phase: switchwire.PhasePrepared,
		}, fixture.agentKeys[index])
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, receipt)
	}
	entries := make([]switchwire.Entry, 0, len(receipts))
	for index, receipt := range receipts {
		encoded, err := switchwire.MarshalEnvelope(receipt)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, switchwire.Entry{ID: fixture.certificate.Agents[index].ID, Key: fixture.certificate.Agents[index].SigningKey, Data: encoded})
	}
	armRequest, err := switchwire.SignFrame(switchwire.Frame{
		Operation: switchwire.OperationArm, Signer: "owner", State: prepared.State, Phase: switchwire.PhaseArmed, Entries: entries,
	}, fixture.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transitions.Arm(armRequest)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := switchwire.VerifyEnvelope(response, map[string]ed25519.PublicKey{fixture.certificate.Agents[0].ID: fixture.agentKeys[0].Public().(ed25519.PublicKey)})
	if err != nil || frame.Phase != switchwire.PhaseArmed {
		t.Fatalf("arm receipt phase=%d error=%v", frame.Phase, err)
	}
	state, err := fixture.journal.Snapshot()
	if err != nil || state.Phase != uint8(switchwire.PhaseArmed) {
		t.Fatalf("durable phase=%d error=%v", state.Phase, err)
	}
}

type transitionFixture struct {
	journal         *Journal
	ownerKey        ed25519.PrivateKey
	agentKeys       [3]ed25519.PrivateKey
	identities      [3]*age.X25519Identity
	certificate     switchwire.Certificate
	enrollment      switchwire.AdmittedEnrollment
	material        switchcontent.Material
	authorityConfig Config
}

func newTransitionFixture(t *testing.T) transitionFixture {
	t.Helper()
	ownerPublic, ownerPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fixture := transitionFixture{ownerKey: ownerPrivate}
	fixture.certificate = switchwire.Certificate{Interval: 3600, Grace: 60, Version: switchwire.Version}
	copy(fixture.certificate.OwnerKey[:], ownerPublic)
	for index := range fixture.agentKeys {
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		fixture.agentKeys[index] = private
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		fixture.identities[index] = identity
		fixture.certificate.Agents[index] = switchwire.Agent{
			ID: []string{"agent-a", "agent-b", "agent-c"}[index], Recipient: identity.Recipient().String(),
			Endpoint: []string{"https://agent-a.example", "https://agent-b.example", "https://agent-c.example"}[index],
		}
		copy(fixture.certificate.Agents[index].SigningKey[:], public)
	}
	recipients := [3]string{}
	for index, agent := range fixture.certificate.Agents {
		recipients[index] = agent.Recipient
	}
	fixture.material, err = switchcontent.Prepare(strings.NewReader("ciphertext object"), recipients)
	if err != nil {
		t.Fatal(err)
	}
	fixture.certificate.Key = fixture.material.KeyID
	fixture.certificate.Content = fixture.material.ContentDigest
	body, err := switchwire.EncodeCertificate(fixture.certificate)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := switchwire.AdmitCertificate(body, ed25519.Sign(ownerPrivate, body), ownerPublic)
	if err != nil {
		t.Fatal(err)
	}
	fixture.enrollment = enrollment
	fixture.authorityConfig = authorityTestConfig(t)
	if err := Initialize(fixture.authorityConfig); err != nil {
		t.Fatal(err)
	}
	fixture.journal, err = Open(fixture.authorityConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.journal.Close() })
	return fixture
}

func (f transitionFixture) config(index int) TransitionConfig {
	body, _ := switchwire.EncodeCertificate(f.certificate)
	ownerPublic := f.ownerKey.Public().(ed25519.PublicKey)
	return TransitionConfig{
		Journal: f.journal, OwnerKey: ownerPublic, EnrollmentBody: body,
		EnrollmentSignature: ed25519.Sign(f.ownerKey, body), AgentID: f.certificate.Agents[index].ID,
		SigningKey: f.agentKeys[index], AgeIdentity: f.identities[index].String(),
		ReadClock: func() (Reading, error) { return Reading{lower: 1000}, nil },
	}
}

func (f transitionFixture) prepareRequest(t *testing.T, ciphertext []byte) switchwire.Envelope {
	return f.prepareRequestAt(t, ciphertext, 4600)
}

func (f transitionFixture) prepareRequestAt(t *testing.T, ciphertext []byte, deadline int64) switchwire.Envelope {
	t.Helper()
	entries := make([]switchwire.Entry, 0, 3)
	for index, agent := range f.certificate.Agents {
		entries = append(entries, switchwire.Entry{ID: agent.ID, Key: sha256.Sum256([]byte(agent.Recipient)), Data: f.material.Shares[index]})
	}
	state := switchwire.State{Switch: "switch-1", Version: switchwire.Version, Sequence: 1, Deadline: deadline, Grace: 60, Content: sha256.Sum256(ciphertext), Key: f.certificate.Key, Enrollment: f.enrollment.Digest}
	request, err := switchwire.SignFrame(switchwire.Frame{Operation: switchwire.OperationPrepare, Signer: "owner", State: state, Phase: switchwire.PhasePrepared, Entries: entries}, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	return request
}
