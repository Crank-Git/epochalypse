package agent

import (
	"bytes"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestRetryRejectsChangedAuthorityWithoutNewReceipt(t *testing.T) {
	for _, operation := range []string{"prepare", "arm"} {
		for name, damage := range map[string]func(*State){
			"content":    func(s *State) { s.Ciphertext[0] ^= 1 },
			"enrollment": func(s *State) { s.Enrollment = []byte("other enrollment") },
			"share":      func(s *State) { s.EncryptedShare = []byte("other share") },
			"request":    func(s *State) { s.AcceptedRequest = []byte("other intent") },
			"prepare":    func(s *State) { s.PrepareRequest = []byte("other prepare") },
			"receipt":    func(s *State) { s.Receipt = []byte("other receipt") },
		} {
			t.Run(operation+"/"+name, func(t *testing.T) {
				fixture, transitions, request := retryFailureFixture(t, operation)
				state, err := fixture.journal.Snapshot()
				if err != nil {
					t.Fatal(err)
				}
				damage(&state)
				state, err = transitions.commit(state)
				if err != nil {
					t.Fatal(err)
				}
				var receipt switchwire.Envelope
				if operation == "prepare" {
					receipt, err = transitions.Prepare(request, fixture.material.Ciphertext, nil)
				} else {
					receipt, err = transitions.Arm(request)
				}
				if err == nil || len(receipt.Body) != 0 {
					t.Fatal("retry issues a receipt for changed authority")
				}
				after, err := fixture.journal.Snapshot()
				if err != nil || after.Generation != state.Generation {
					t.Fatalf("rejected retry changes authority: %v", err)
				}
			})
		}
	}
}

func retryFailureFixture(t *testing.T, operation string) (transitionFixture, *Transitions, switchwire.Envelope) {
	t.Helper()
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	if operation == "arm" {
		prepareAndArmFixture(t, fixture, transitions)
	} else if _, err := transitions.Prepare(fixture.prepareRequest(t, fixture.material.Ciphertext), fixture.material.Ciphertext, nil); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	request, err := switchwire.ReadEnvelope(bytes.NewReader(state.AcceptedRequest))
	if err != nil {
		t.Fatal(err)
	}
	return fixture, transitions, request
}

func TestRetryCompletesReceiptAfterAcceptedCommit(t *testing.T) {
	for _, operation := range []string{"prepare", "arm"} {
		t.Run(operation, func(t *testing.T) {
			fixture, transitions, request := retryFailureFixture(t, operation)
			state, err := fixture.journal.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			state.Receipt = nil
			if _, err := transitions.commit(state); err != nil {
				t.Fatal(err)
			}
			if operation == "prepare" {
				_, err = transitions.Prepare(request, fixture.material.Ciphertext, nil)
			} else {
				_, err = transitions.Arm(request)
			}
			if err != nil {
				t.Fatalf("retry cannot complete committed acceptance: %v", err)
			}
			state, err = fixture.journal.Snapshot()
			if err != nil || len(state.Receipt) == 0 {
				t.Fatalf("retry returns before receipt persistence: %v", err)
			}
		})
	}
}
