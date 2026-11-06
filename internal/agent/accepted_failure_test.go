package agent

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestPrepareRejectsUnavailableClockAndInvalidIssueRange(t *testing.T) {
	for _, scenario := range []string{"no clock", "negative issue"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			config := fixture.config(0)
			deadline := int64(4600)
			switch scenario {
			case "no clock":
				config.ReadClock = nil
			case "negative issue":
				deadline = 100
			}
			transitions, err := NewTransitions(config)
			if err != nil {
				t.Fatal(err)
			}
			request := fixture.prepareRequestAt(t, fixture.material.Ciphertext, deadline)
			receipt, err := transitions.Prepare(request, fixture.material.Ciphertext, nil)
			if !errors.Is(err, ErrClock) || len(receipt.Body) != 0 {
				t.Fatalf("invalid clock boundary returns receipt: %v", err)
			}
			state, err := fixture.journal.Snapshot()
			if err != nil || state.Generation != 0 {
				t.Fatalf("invalid boundary changes authority: %v", err)
			}
		})
	}
}

func TestRetryRejectsReceiptForOtherPhase(t *testing.T) {
	for _, operation := range []string{"prepare", "arm"} {
		t.Run(operation, func(t *testing.T) {
			fixture, transitions, request := retryFailureFixture(t, operation)
			state, err := fixture.journal.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := switchwire.ReadEnvelope(bytes.NewReader(state.Receipt))
			if err != nil {
				t.Fatal(err)
			}
			frame, err := switchwire.DecodeFrame(receipt.Body)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "prepare" {
				frame.Phase = switchwire.PhaseArmed
			} else {
				frame.Phase = switchwire.PhasePrepared
			}
			receipt, err = switchwire.SignFrame(frame, fixture.agentKeys[0])
			if err != nil {
				t.Fatal(err)
			}
			state.Receipt, err = switchwire.MarshalEnvelope(receipt)
			if err != nil {
				t.Fatal(err)
			}
			state, err = transitions.commit(state)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "prepare" {
				receipt, err = transitions.Prepare(request, fixture.material.Ciphertext, nil)
			} else {
				receipt, err = transitions.Arm(request)
			}
			if !errors.Is(err, ErrTransitionConflict) || len(receipt.Body) != 0 {
				t.Fatalf("retry trusts another phase: %v", err)
			}
			after, err := fixture.journal.Snapshot()
			if err != nil || after.Generation != state.Generation {
				t.Fatalf("rejection changes authority: %v", err)
			}
		})
	}
}

func TestArmRejectsDamagedPreparedAuthorityBeforeClock(t *testing.T) {
	for name, damage := range map[string]func(*State){
		"missing phase":     func(s *State) { s.Phase = 0 },
		"enrollment":        func(s *State) { s.Enrollment = nil },
		"request encoding":  func(s *State) { s.AcceptedRequest = []byte("broken") },
		"request signature": func(s *State) { s.AcceptedRequest[len(s.AcceptedRequest)-1] ^= 1 },
		"content":           func(s *State) { s.Ciphertext[0] ^= 1 },
		"missing receipt":   func(s *State) { s.Receipt = nil },
		"receipt encoding":  func(s *State) { s.Receipt = []byte("broken") },
	} {
		t.Run(name, func(t *testing.T) {
			fixture, transitions, arm := retryFailureFixture(t, "arm")
			state, err := fixture.journal.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			state.Phase = switchwire.PhasePrepared
			state.AcceptedRequest = bytes.Clone(state.PrepareRequest)
			frame, err := switchwire.DecodeFrame(arm.Body)
			if err != nil {
				t.Fatal(err)
			}
			state.Receipt = bytes.Clone(frame.Entries[0].Data)
			damage(&state)
			state, err = transitions.commit(state)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			transitions.clock = func() (Reading, error) { calls++; return Reading{lower: 1000}, nil }
			receipt, err := transitions.Arm(arm)
			if err == nil || len(receipt.Body) != 0 || calls != 0 {
				t.Fatalf("damaged authority reaches receipt or clock: %v", err)
			}
			after, err := fixture.journal.Snapshot()
			if err != nil || after.Generation != state.Generation {
				t.Fatalf("rejection changes authority: %v", err)
			}
		})
	}
}

func TestRetryWithholdsReceiptWhenClockLatchCannotPersist(t *testing.T) {
	for _, operation := range []string{"prepare", "arm"} {
		t.Run(operation, func(t *testing.T) {
			fixture, transitions, request := retryFailureFixture(t, operation)
			fault := errors.New("synthetic clock latch persistence failure")
			transitions.clock = func() (Reading, error) { return Reading{}, ErrClock }
			installDurabilityFault(fixture.journal, "sync-record-body", fault)
			var receipt switchwire.Envelope
			var err error
			if operation == "prepare" {
				receipt, err = transitions.Prepare(request, fixture.material.Ciphertext, nil)
			} else {
				receipt, err = transitions.Arm(request)
			}
			if !errors.Is(err, ErrClock) || !errors.Is(err, fault) || len(receipt.Body) != 0 {
				t.Fatalf("clock failure loses cause or returns receipt: %v", err)
			}
			if _, err := fixture.journal.Snapshot(); !errors.Is(err, ErrQuarantined) {
				t.Fatalf("uncertain clock latch remains available: %v", err)
			}
		})
	}
}

func TestPrepareRequiresInitialSequenceAndLocalShare(t *testing.T) {
	for _, change := range []string{"sequence", "share", "identity"} {
		t.Run(change, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			transitions, err := NewTransitions(fixture.config(0))
			if err != nil {
				t.Fatal(err)
			}
			request := fixture.prepareRequest(t, fixture.material.Ciphertext)
			frame, err := switchwire.DecodeFrame(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "sequence":
				frame.State.Sequence = 2
			case "share":
				frame.Entries[0].Data = []byte("invalid encrypted share")
			case "identity":
				transitions.agentID = "unregistered-agent"
			}
			request, err = switchwire.SignFrame(frame, fixture.ownerKey)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := transitions.Prepare(request, fixture.material.Ciphertext, nil)
			if err == nil || len(receipt.Body) != 0 {
				t.Fatal("invalid prepare returns a receipt")
			}
			state, err := fixture.journal.Snapshot()
			if err != nil || state.Generation != 0 {
				t.Fatalf("invalid prepare changes authority: %v", err)
			}
		})
	}
}
