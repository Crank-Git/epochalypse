package agent

import (
	"errors"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestCheckinWithholdsReceiptAfterAuthorityOrClockFailure(t *testing.T) {
	for _, kind := range []string{"closed authority", "unavailable clock", "intent sync"} {
		t.Run(kind, func(t *testing.T) {
			f, tr, frame := armedCheckinFixture(t)
			request, err := switchwire.SignFrame(frame, f.ownerKey)
			if err != nil {
				t.Fatal(err)
			}
			cause := errors.New("synthetic check-in prerequisite failure")
			switch kind {
			case "closed authority":
				if err := f.journal.Close(); err != nil {
					t.Fatal(err)
				}
			case "unavailable clock":
				tr.clock = func() (Reading, error) { return Reading{}, cause }
			case "intent sync":
				f.journal.performIO = func(operation string, action func() error) error {
					if operation == "seek-journal" {
						return cause
					}
					return action()
				}
			}
			receipt, err := tr.Checkin(request)
			if err == nil || len(receipt.Body) != 0 {
				t.Fatal("failed prerequisite returns a receipt")
			}
			if kind == "intent sync" && !errors.Is(err, cause) {
				t.Fatalf("durability failure loses cause: %v", err)
			}
		})
	}
}

func TestCheckinRejectsPreparedAuthority(t *testing.T) {
	f := newTransitionFixture(t)
	tr, err := NewTransitions(f.config(0))
	if err != nil {
		t.Fatal(err)
	}
	prepare := f.prepareRequest(t, f.material.Ciphertext)
	if _, err := tr.Prepare(prepare, f.material.Ciphertext, nil); err != nil {
		t.Fatal(err)
	}
	frame, err := switchwire.DecodeFrame(prepare.Body)
	if err != nil {
		t.Fatal(err)
	}
	frame.Operation, frame.Phase, frame.Entries = switchwire.OperationCheckin, switchwire.PhaseArmed, nil
	frame.State.Sequence++
	request, err := switchwire.SignFrame(frame, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := tr.Checkin(request); !errors.Is(err, ErrTransitionConflict) || len(receipt.Body) != 0 {
		t.Fatalf("unarmed authority renews: %v", err)
	}
}

func TestRenewedAuthorityRejectsInvalidOriginalPreparation(t *testing.T) {
	f, tr, frame := armedCheckinFixture(t)
	request, err := switchwire.SignFrame(frame, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Checkin(request); err != nil {
		t.Fatal(err)
	}
	state, err := f.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.validateRenewedMaterial(state, frame); err != nil {
		t.Fatalf("valid renewed authority: %v", err)
	}
	for _, prepare := range [][]byte{[]byte("corrupt original preparation"), state.AcceptedRequest} {
		bad := state
		bad.PrepareRequest = prepare
		if err := tr.validateRenewedMaterial(bad, frame); err == nil {
			t.Fatal("invalid original preparation authenticates renewed authority")
		}
	}
}
