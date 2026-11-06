package agent

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"math"
	"os"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestValidateArmReceiptsRejectsMalformedLocalReceipt(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	if err := transitions.validateArmReceipts([]byte("invalid envelope"), switchwire.Frame{}); err == nil {
		t.Fatal("arm accepted a malformed local receipt")
	}
}

func TestValidateArmReceiptsRequiresLocalReceipt(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	prepared, err := switchwire.VerifyOwnerRequest(request, fixture.enrollment)
	if err != nil {
		t.Fatal(err)
	}
	otherReceipt, err := switchwire.SignFrame(switchwire.Frame{
		Operation: switchwire.OperationReceipt, Signer: "agent-b", State: prepared.State,
		Durable: prepared.State.Sequence, Phase: switchwire.PhasePrepared,
	}, fixture.agentKeys[1])
	if err != nil {
		t.Fatal(err)
	}
	localReceipt, err := switchwire.SignFrame(switchwire.Frame{
		Operation: switchwire.OperationReceipt, Signer: "agent-a", State: prepared.State,
		Durable: prepared.State.Sequence, Phase: switchwire.PhasePrepared,
	}, fixture.agentKeys[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := transitions.validateArmReceipts(envelopeBytes(t, localReceipt), switchwire.Frame{Entries: []switchwire.Entry{{Data: envelopeBytes(t, otherReceipt)}}}); !errors.Is(err, ErrTransitionConflict) {
		t.Fatalf("arm receipt without the local receipt error = %v", err)
	}
}

func TestValidateArmReceiptsRequiresThreeConfirmedReceipts(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	prepared, err := switchwire.VerifyOwnerRequest(request, fixture.enrollment)
	if err != nil {
		t.Fatal(err)
	}
	localReceipt, err := switchwire.SignFrame(switchwire.Frame{
		Operation: switchwire.OperationReceipt, Signer: "agent-a", State: prepared.State,
		Durable: prepared.State.Sequence, Phase: switchwire.PhasePrepared,
	}, fixture.agentKeys[0])
	if err != nil {
		t.Fatal(err)
	}
	data := envelopeBytes(t, localReceipt)
	if err := transitions.validateArmReceipts(data, switchwire.Frame{State: prepared.State, Entries: []switchwire.Entry{{Data: data}}}); err == nil {
		t.Fatal("arm accepted fewer than three confirmed receipts")
	}
}

func TestValidateStoredPrepareUsesAcceptedRequestWhenPrepareRequestIsEmpty(t *testing.T) {
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
	state.PrepareRequest = nil
	frame, err := switchwire.VerifyOwnerRequest(request, fixture.enrollment)
	if err != nil {
		t.Fatal(err)
	}
	if err := transitions.validateStoredPrepare(state, frame.State); err != nil {
		t.Fatalf("stored prepare did not use its accepted request: %v", err)
	}
}

func TestNewTransitionsRejectsInvalidAgeIdentity(t *testing.T) {
	fixture := newTransitionFixture(t)
	config := fixture.config(0)
	config.AgeIdentity = "invalid age identity"
	if _, err := NewTransitions(config); err == nil {
		t.Fatal("constructor accepted an invalid age identity")
	}
}

func TestValidateReceiptRejectsReceiptFromAnotherAgent(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	prepared, err := switchwire.VerifyOwnerRequest(request, fixture.enrollment)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := switchwire.SignFrame(switchwire.Frame{
		Operation: switchwire.OperationReceipt, Signer: "agent-b", State: prepared.State,
		Durable: prepared.State.Sequence, Phase: switchwire.PhasePrepared,
	}, fixture.agentKeys[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := transitions.validateReceipt(envelopeBytes(t, receipt), prepared.State, switchwire.PhasePrepared); !errors.Is(err, ErrTransitionConflict) {
		t.Fatalf("receipt from another agent error = %v", err)
	}
}

func TestValidateStoredContentRejectsChangedCiphertext(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	prepared, err := switchwire.VerifyOwnerRequest(request, fixture.enrollment)
	if err != nil {
		t.Fatal(err)
	}
	state := State{Ciphertext: bytes.Clone(fixture.material.Ciphertext), Enrollment: transitions.enrollmentB, EncryptedShare: fixture.material.Shares[0]}
	state.Ciphertext[len(state.Ciphertext)-1] ^= 1
	if err := transitions.validateStoredContent(state, prepared.State); !errors.Is(err, ErrTransitionConflict) {
		t.Fatalf("changed stored ciphertext error = %v", err)
	}
}

func TestValidateAcceptedArmRejectsMalformedStoredRequest(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.prepareRequest(t, fixture.material.Ciphertext)
	prepared, err := switchwire.VerifyOwnerRequest(request, fixture.enrollment)
	if err != nil {
		t.Fatal(err)
	}
	err = transitions.validateAcceptedArm([]byte("invalid envelope"), prepared.State)
	if err == nil || errors.Is(err, ErrTransitionConflict) {
		t.Fatalf("malformed stored arm request error = %v", err)
	}
}

func TestCheckClockRejectsMalformedStoredRequests(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	for name, requestData := range map[string][]byte{
		"invalid envelope": []byte("bad"),
		"invalid frame":    envelopeBytes(t, switchwire.Envelope{Body: []byte("bad frame"), Signature: make([]byte, ed25519.SignatureSize)}),
	} {
		t.Run(name, func(t *testing.T) {
			if err := transitions.checkClock(State{AcceptedRequest: requestData}); err == nil {
				t.Fatal("clock check accepted malformed retained request")
			}
		})
	}
}

func TestCheckClockForBoundaryRejectsDeadlineBeforeEnrollmentInterval(t *testing.T) {
	fixture := newTransitionFixture(t)
	config := fixture.config(0)
	config.ReadClock = func() (Reading, error) { return Reading{lower: 0}, nil }
	transitions, err := NewTransitions(config)
	if err != nil {
		t.Fatal(err)
	}
	frame := switchwire.Frame{State: switchwire.State{Deadline: 3599}}
	if err := transitions.checkClockForBoundary(frame, State{}); !errors.Is(err, ErrClock) {
		t.Fatalf("deadline before enrollment interval error = %v", err)
	}
}

func TestTransitionCommitReturnsSnapshotFailure(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := transitions.commit(State{}); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("commit snapshot error = %v", err)
	}
}

func TestTransitionCommitRejectsMaximumGeneration(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	fixture.journal.mu.Lock()
	fixture.journal.state.Generation = math.MaxUint64
	fixture.journal.mu.Unlock()
	if _, err := transitions.commit(State{}); !errors.Is(err, ErrGeneration) {
		t.Fatalf("maximum generation error = %v", err)
	}
}

func envelopeBytes(t *testing.T, envelope switchwire.Envelope) []byte {
	t.Helper()
	data, err := switchwire.MarshalEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
