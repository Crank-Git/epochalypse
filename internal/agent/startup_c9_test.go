package agent

import (
	"errors"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestServiceRejectsAuthorityPhaseWithoutMatchingMaterial(t *testing.T) {
	for _, scenario := range []string{"empty phase with material", "stopped phase", "armed phase with prepare request"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			transitions, err := NewTransitions(fixture.config(0))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := transitions.Prepare(fixture.prepareRequest(t, fixture.material.Ciphertext), fixture.material.Ciphertext, nil); err != nil {
				t.Fatal(err)
			}
			state, err := fixture.journal.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "empty phase with material":
				state.Phase = 0
			case "stopped phase":
				state.Phase = switchwire.PhaseStopped
			case "armed phase with prepare request":
				state.Phase = switchwire.PhaseArmed
			}
			state, err = transitions.commit(state)
			if err != nil {
				t.Fatal(err)
			}
			if err := fixture.journal.Close(); err != nil {
				t.Fatal(err)
			}
			clockCalls := 0
			config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
			config.Transition.ReadClock = func() (Reading, error) { clockCalls++; return Reading{lower: 1000}, nil }
			service, err := OpenService(config)
			if service != nil {
				_ = service.Close()
			}
			if !errors.Is(err, ErrQuarantined) || clockCalls != 0 {
				t.Fatalf("inconsistent phase grants readiness: %v, clock calls=%d", err, clockCalls)
			}
			reopened, err := Open(fixture.authorityConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = reopened.Close() }()
			got, err := reopened.Snapshot()
			if err != nil || !equalAuthorityState(state, got) {
				t.Fatalf("rejected startup changes retained authority: %+v, %v", got, err)
			}
		})
	}
}
