package agent

import (
	"errors"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestPrepareWithholdsReceiptAfterDurabilityFailure(t *testing.T) {
	for _, operation := range []string{
		"seek-journal", "write-record-length", "write-record-body", "write-record-checksum",
		"sync-record-body", "write-commit-marker", "sync-commit-marker",
		"create-checkpoint-directory", "open-checkpoint-root", "create-checkpoint-file",
		"protect-checkpoint-file", "write-checkpoint", "sync-checkpoint", "close-checkpoint",
		"rename-checkpoint", "sync-checkpoint-directory",
	} {
		t.Run(operation, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			fault := errors.New("synthetic durability failure")
			installDurabilityFault(fixture.journal, operation, fault)
			transitions, err := NewTransitions(fixture.config(0))
			if err != nil {
				t.Fatal(err)
			}
			request := fixture.prepareRequest(t, fixture.material.Ciphertext)
			receipt, err := transitions.Prepare(request, fixture.material.Ciphertext, nil)
			if !errors.Is(err, fault) || len(receipt.Body) != 0 || len(receipt.Signature) != 0 {
				t.Fatalf("durability failure returns receipt or loses cause: %v", err)
			}
			if _, err := fixture.journal.Snapshot(); operation != "seek-journal" && !errors.Is(err, ErrQuarantined) {
				t.Fatalf("uncertain durability remains available: %v", err)
			}
			if err := fixture.journal.Close(); err != nil {
				t.Fatal(err)
			}
			assertDurabilityFailureRecovery(t, fixture, request)
		})
	}
}

func assertDurabilityFailureRecovery(t *testing.T, fixture transitionFixture, request switchwire.Envelope) {
	t.Helper()
	reopened, err := Open(fixture.authorityConfig)
	if errors.Is(err, ErrQuarantined) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	state, err := reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != 0 && state.Phase != switchwire.PhasePrepared {
		t.Fatalf("failed prepare changes phase: %d", state.Phase)
	}
	if len(state.Receipt) != 0 {
		t.Fatal("failed durability persists a receipt")
	}
	config := fixture.config(0)
	config.Journal = reopened
	retry, err := NewTransitions(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retry.Prepare(request, fixture.material.Ciphertext, nil); err != nil {
		t.Fatalf("intact authority cannot retry exact prepare: %v", err)
	}
}

func installDurabilityFault(journal *Journal, operation string, fault error) {
	journal.performIO = func(name string, action func() error) error {
		if name != operation {
			return action()
		}
		// Close can report a delayed write error after it releases the descriptor.
		if name == "close-checkpoint" {
			return errors.Join(action(), fault)
		}
		return fault
	}
}
