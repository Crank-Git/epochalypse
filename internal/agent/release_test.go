package agent

import (
	"bytes"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestPublicRecoveryWithholdsPrivatePrepareMaterial(t *testing.T) {
	f := newTransitionFixture(t)
	tr, err := NewTransitions(f.config(0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Prepare(f.prepareRequest(t, f.material.Ciphertext), f.material.Ciphertext, nil); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "/ciphertext"} {
		w := httptest.NewRecorder()
		tr.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/agent/v1/public/switches/switch-1"+suffix, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("prepared public status = %d", w.Code)
		}
	}
}

func TestReleasedServiceRestartsWithoutClockAuthority(t *testing.T) {
	f, tr, renewal := armedCheckinFixture(t)
	tr.clock = func() (Reading, error) { return Reading{lower: 4660}, nil }
	if err := tr.Publish(); err != nil {
		t.Fatal(err)
	}
	if err := f.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{Authority: f.authorityConfig, Transition: f.config(0), LabOnlySyntheticClock: true}
	clockCalls := 0
	config.Transition.ReadClock = func() (Reading, error) { clockCalls++; return Reading{}, ErrClock }
	s, err := OpenService(config)
	if err != nil {
		t.Fatalf("released restart requires fresh clock: %v", err)
	}
	defer func() { _ = s.Close() }()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/agent/v1/public/switches/switch-1", nil))
	if w.Code != http.StatusOK || clockCalls != 0 {
		t.Fatalf("released public status = %d, clock calls = %d", w.Code, clockCalls)
	}
	report := s.Readiness()
	if !report.RecoveryAvailable || report.Ready || report.ClockReady {
		t.Fatalf("released readiness: %+v", report)
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/agent/v1/public/switches/switch-1", nil))
	if w.Code != http.StatusOK {
		t.Fatal("metadata blocks released recovery")
	}
	state, err := s.journal.Snapshot()
	if err != nil || state.UnsafeClock || !state.PossibleDisclosure {
		t.Fatalf("released restart state: %v", err)
	}
	request, err := switchwire.SignFrame(renewal, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.transitions.Checkin(request); err == nil {
		t.Fatal("released restart accepts renewal")
	}
}

func TestPublicReleasePersistsDisclosureAtAcceptedBoundary(t *testing.T) {
	f, tr, renewal := armedCheckinFixture(t)
	now := int64(4659)
	tr.clock = func() (Reading, error) { return Reading{lower: now}, nil }
	get := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		tr.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/agent/v1/public/switches/switch-1", nil))
		return w
	}
	before := get()
	if before.Code != http.StatusOK {
		t.Fatalf("public manifest status = %d", before.Code)
	}
	state, err := f.journal.Snapshot()
	if err != nil || state.PossibleDisclosure || len(state.ReleaseEnvelope) != 0 {
		t.Fatal("share precedes accepted boundary")
	}
	now++
	after := get()
	if after.Code != http.StatusOK {
		t.Fatalf("public release status = %d", after.Code)
	}
	state, err = f.journal.Snapshot()
	if err != nil || !state.PossibleDisclosure || len(state.ReleaseEnvelope) == 0 {
		t.Fatal("public share lacks durable irreversible history")
	}
	assertReleasedHistory(t, f, tr, renewal, state)
}

func assertReleasedHistory(t *testing.T, f transitionFixture, tr *Transitions, renewal switchwire.Frame, state State) {
	t.Helper()
	envelope, err := switchwire.ReadEnvelope(bytes.NewReader(state.ReleaseEnvelope))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := switchwire.VerifyEnvelope(envelope, map[string]ed25519.PublicKey{tr.agentID: f.agentKeys[0].Public().(ed25519.PublicKey)})
	if err != nil || frame.Operation != switchwire.OperationRelease || !frame.Released || len(frame.Entries) != 1 {
		t.Fatal("release lacks authenticated share")
	}
	renewal.State.Deadline = 8200
	signed, err := switchwire.SignFrame(renewal, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Checkin(signed); err == nil {
		t.Fatal("released content key accepts renewal")
	}
	if err := f.journal.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(f.authorityConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	recovered, err := reopened.Snapshot()
	if err != nil || !recovered.PossibleDisclosure || !bytes.Equal(recovered.ReleaseEnvelope, state.ReleaseEnvelope) {
		t.Fatal("restart loses release history")
	}
}
