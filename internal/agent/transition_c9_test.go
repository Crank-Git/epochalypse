package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestArmRejectsUnauthenticatedAndWrongOperationRequests(t *testing.T) {
	for _, scenario := range []string{"invalid signature", "prepare operation"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			transitions, err := NewTransitions(fixture.config(0))
			if err != nil {
				t.Fatal(err)
			}
			request := fixture.prepareRequest(t, fixture.material.Ciphertext)
			if scenario == "invalid signature" {
				request.Signature = bytes.Clone(request.Signature)
				request.Signature[0] ^= 1
			}
			_, err = transitions.Arm(request)
			if err == nil {
				t.Fatal("arm accepted an unauthenticated request")
			}
			state, snapshotErr := fixture.journal.Snapshot()
			if snapshotErr != nil || state.Generation != 0 || state.Phase != 0 {
				t.Fatalf("rejected arm changed authority: state=%+v error=%v", state, snapshotErr)
			}
		})
	}
}

func TestPrepareRejectsInvalidContentAndManifestBeforeClock(t *testing.T) {
	for _, scenario := range []string{"empty ciphertext", "changed ciphertext", "unexpected manifest"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			calls := 0
			config := fixture.config(0)
			config.ReadClock = func() (Reading, error) { calls++; return Reading{lower: 1000}, nil }
			transitions, err := NewTransitions(config)
			if err != nil {
				t.Fatal(err)
			}
			ciphertext := bytes.Clone(fixture.material.Ciphertext)
			manifest := []byte(nil)
			switch scenario {
			case "empty ciphertext":
				ciphertext = nil
			case "changed ciphertext":
				ciphertext[len(ciphertext)-1] ^= 1
			case "unexpected manifest":
				manifest = []byte("manifest")
			}
			if receipt, callErr := transitions.Prepare(fixture.prepareRequest(t, fixture.material.Ciphertext), ciphertext, manifest); callErr == nil || len(receipt.Body) != 0 {
				t.Fatalf("invalid content returned a receipt: %v", callErr)
			}
			state, err := fixture.journal.Snapshot()
			if err != nil || state.Generation != 0 || calls != 0 {
				t.Fatalf("invalid content reached clock or authority: generation=%d calls=%d error=%v", state.Generation, calls, err)
			}
		})
	}
}

func TestTransitionHandlerRejectsMethodAndMalformedBodies(t *testing.T) {
	for _, scenario := range []struct {
		name, method, path, body string
		status                   int
	}{
		{name: "wrong method", method: http.MethodGet, path: "/agent/v1/switches/switch-1/prepare", status: http.StatusMethodNotAllowed},
		{name: "unknown operation", method: http.MethodPost, path: "/agent/v1/switches/switch-1/export", status: http.StatusNotFound},
		{name: "malformed JSON", method: http.MethodPost, path: "/agent/v1/switches/switch-1/prepare", body: "{", status: http.StatusBadRequest},
		{name: "unknown JSON field", method: http.MethodPost, path: "/agent/v1/switches/switch-1/prepare", body: `{"unexpected":true}`, status: http.StatusBadRequest},
		{name: "trailing JSON", method: http.MethodPost, path: "/agent/v1/switches/switch-1/prepare", body: `{} {}`, status: http.StatusBadRequest},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			transitions, err := NewTransitions(fixture.config(0))
			if err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(scenario.method, scenario.path, strings.NewReader(scenario.body))
			transitions.Handler().ServeHTTP(recorder, request)
			if recorder.Code != scenario.status {
				t.Fatalf("handler status = %d, want %d", recorder.Code, scenario.status)
			}
			state, err := fixture.journal.Snapshot()
			if err != nil || state.Generation != 0 {
				t.Fatalf("invalid HTTP request changed authority: generation=%d error=%v", state.Generation, err)
			}
		})
	}
}

func TestPrepareRetryRecreatesReceiptAfterJournalRestart(t *testing.T) {
	fixture := newTransitionFixture(t)
	config := fixture.config(0)
	transitions, err := NewTransitions(config)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	first, err := transitions.Prepare(request, fixture.material.Ciphertext, nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	state.Receipt = nil
	if _, err := transitions.commit(state); err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.journal, err = Open(fixture.authorityConfig)
	if err != nil {
		t.Fatal(err)
	}
	config.Journal = fixture.journal
	transitions, err = NewTransitions(config)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := transitions.Prepare(request, fixture.material.Ciphertext, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Body, retried.Body) || !bytes.Equal(first.Signature, retried.Signature) {
		t.Fatal("receipt recovery changed the signed response")
	}
	retained, err := fixture.journal.Snapshot()
	if err != nil || retained.Phase != uint8(switchwire.PhasePrepared) || len(retained.Receipt) == 0 || retained.Generation <= state.Generation {
		t.Fatalf("receipt recovery did not retain accepted state: %+v error=%v", retained, err)
	}
}

func TestOpenServiceRejectsMissingAuthorityWithoutCreatingIt(t *testing.T) {
	fixture := newTransitionFixture(t)
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
	config.Authority.ExpectedVolumeID = "different-volume"
	service, err := OpenService(config)
	if service != nil {
		_ = service.Close()
	}
	if err == nil {
		t.Fatal("service opened an authority with a different pinned identity")
	}
	if _, err := Open(config.Authority); !errors.Is(err, ErrVolumeIdentity) {
		t.Fatalf("failed startup changed authority: %v", err)
	}
}

func TestTransitionHandlerRejectsSwitchMismatchAndHidesRejectedRequest(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(transitionRequest{Request: fixture.prepareRequest(t, fixture.material.Ciphertext), Ciphertext: fixture.material.Ciphertext})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/agent/v1/switches/other/prepare", bytes.NewReader(input))
	transitions.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || strings.Contains(recorder.Body.String(), "Signature") {
		t.Fatalf("switch mismatch response status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	state, err := fixture.journal.Snapshot()
	if err != nil || state.Generation != 0 {
		t.Fatalf("switch mismatch changed authority: generation=%d error=%v", state.Generation, err)
	}
}

func TestPrepareRejectsGraceDeadlineBoundaryWithoutReceipt(t *testing.T) {
	fixture := newTransitionFixture(t)
	config := fixture.config(0)
	config.ReadClock = func() (Reading, error) { return Reading{lower: 4660}, nil }
	transitions, err := NewTransitions(config)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequestAt(t, fixture.material.Ciphertext, 4600)
	if receipt, err := transitions.Prepare(request, fixture.material.Ciphertext, nil); !errors.Is(err, ErrClock) || len(receipt.Body) != 0 {
		t.Fatalf("prepare at the grace boundary returned receipt or wrong error: %v", err)
	}
	state, err := fixture.journal.Snapshot()
	if err != nil || state.Generation != 0 || state.UnsafeClock {
		t.Fatalf("boundary rejection changed authority: %+v error=%v", state, err)
	}
}

func TestPrepareWithholdsReceiptWhenReceiptPersistenceFails(t *testing.T) {
	fixture := newTransitionFixture(t)
	fault := errors.New("receipt persistence failed")
	receiptSyncFailed := false
	fixture.journal.performIO = func(name string, action func() error) error {
		accepted := fixture.journal.state
		if name == "sync-record-body" && accepted.Phase == switchwire.PhasePrepared && len(accepted.Receipt) == 0 {
			receiptSyncFailed = true
			return fault
		}
		return action()
	}
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := transitions.Prepare(fixture.prepareRequest(t, fixture.material.Ciphertext), fixture.material.Ciphertext, nil)
	if !errors.Is(err, fault) || len(receipt.Body) != 0 || len(receipt.Signature) != 0 {
		t.Fatalf("receipt persistence failure returns receipt or loses cause: %v", err)
	}
	if !receiptSyncFailed {
		t.Fatal("receipt persistence does not reach the durability failure")
	}
	if _, err := fixture.journal.Snapshot(); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("uncertain receipt persistence remains available: %v", err)
	}
}
