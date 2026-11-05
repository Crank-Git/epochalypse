package agent

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestOpenServiceRequiresExplicitLabClockMode(t *testing.T) {
	fixture := newTransitionFixture(t)
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0)}
	if _, err := OpenService(config); err == nil {
		t.Fatal("service opened without an explicit lab clock mode")
	}
}

func TestOpenServicePersistsUnsafeClockForAcceptedMaterial(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	if _, err := transitions.Prepare(request, fixture.material.Ciphertext, nil); err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
	config.Transition.ReadClock = func() (Reading, error) { return Reading{}, ErrClock }
	if _, err := OpenService(config); !errors.Is(err, ErrClock) {
		t.Fatalf("startup error = %v", err)
	}
	journal, err := Open(fixture.authorityConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()
	state, err := journal.Snapshot()
	if err != nil || !state.UnsafeClock {
		t.Fatalf("unsafe clock latch = %t, error = %v", state.UnsafeClock, err)
	}
}

func TestOpenServiceRequiresClockEvidenceBeforeReadiness(t *testing.T) {
	fixture := newTransitionFixture(t)
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{
		Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true,
	}
	config.Transition.ReadClock = func() (Reading, error) { return Reading{}, ErrClock }
	if _, err := OpenService(config); !errors.Is(err, ErrClock) {
		t.Fatalf("service startup error = %v, want ErrClock", err)
	}
}

func TestOpenServiceDoesNotInitializeMissingAuthority(t *testing.T) {
	fixture := newTransitionFixture(t)
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{
		Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true,
	}
	config.Authority.ExpectedVolumeID = "different-volume"
	if _, err := OpenService(config); err == nil {
		t.Fatal("service opened an authority with a different pinned identity")
	}
	if _, err := Open(config.Authority); err == nil {
		t.Fatal("failed service startup changed the pinned authority")
	}
}

func TestOpenServiceExposesHandlerAfterValidation(t *testing.T) {
	fixture := newTransitionFixture(t)
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{
		Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true,
	}
	service, err := OpenService(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Error(err)
		}
	})
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, httptest.NewRequest("POST", "/unknown", nil))
	if response.Code != 404 {
		t.Fatalf("unknown path status = %d, want 404", response.Code)
	}
}

func TestOpenServiceValidatesPreparedAuthorityBeforeClock(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	if _, err := transitions.Prepare(request, fixture.material.Ciphertext, nil); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	state.Ciphertext[0] ^= 1
	if _, err := transitions.commit(state); err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	clockCalls := 0
	config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
	config.Transition.ReadClock = func() (Reading, error) {
		clockCalls++
		return Reading{lower: 1000}, nil
	}
	if _, err := OpenService(config); err == nil {
		t.Fatal("service accepted altered durable ciphertext")
	}
	if clockCalls != 0 {
		t.Fatal("service assessed the clock before it validated durable content")
	}
}

func TestOpenServiceReopensPreparedStateWithoutReceipt(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	if _, err := transitions.Prepare(request, fixture.material.Ciphertext, nil); err != nil {
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
	config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
	service, err := OpenService(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenServiceReopensArmedAuthority(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	prepareAndArmFixture(t, fixture, transitions)
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
	service, err := OpenService(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenServiceRejectsIrreversibleLatchesBeforeClock(t *testing.T) {
	for name, setLatch := range map[string]func(*State){
		"unsafe clock":        func(state *State) { state.UnsafeClock = true },
		"possible disclosure": func(state *State) { state.PossibleDisclosure = true },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			transitions, err := NewTransitions(fixture.config(0))
			if err != nil {
				t.Fatal(err)
			}
			state, err := fixture.journal.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			setLatch(&state)
			if _, err := transitions.commit(state); err != nil {
				t.Fatal(err)
			}
			if err := fixture.journal.Close(); err != nil {
				t.Fatal(err)
			}
			clockCalls := 0
			config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
			config.Transition.ReadClock = func() (Reading, error) {
				clockCalls++
				return Reading{lower: 1000}, nil
			}
			if _, err := OpenService(config); err == nil {
				t.Fatal("service accepted an irreversible state latch")
			}
			if clockCalls != 0 {
				t.Fatal("service assessed the clock before it rejected an irreversible latch")
			}
		})
	}
}

func prepareAndArmFixture(t *testing.T, fixture transitionFixture, transitions *Transitions) {
	t.Helper()
	prepareRequest := fixture.prepareRequest(t, fixture.material.Ciphertext)
	if _, err := transitions.Prepare(prepareRequest, fixture.material.Ciphertext, nil); err != nil {
		t.Fatal(err)
	}
	prepared, err := switchwire.VerifyOwnerRequest(prepareRequest, fixture.enrollment)
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
	if _, err := transitions.Arm(armRequest); err != nil {
		t.Fatal(err)
	}
}
