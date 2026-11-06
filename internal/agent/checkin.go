package agent

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

// Checkin preserves the signed deadline before it returns a durable receipt.
func (t *Transitions) Checkin(request switchwire.Envelope) (switchwire.Envelope, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	frame, encoded, err := t.validateCheckin(request)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	current, prior, err := t.checkinAuthority()
	if err != nil {
		return switchwire.Envelope{}, err
	}
	exact := bytes.Equal(encoded, current.AcceptedRequest)
	if !exact && !validRenewal(prior.State, frame.State) {
		return switchwire.Envelope{}, ErrTransitionConflict
	}
	reading, err := t.assessClock(current)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	// Both boundaries use one fresh assessment so a late request cannot restore eligibility.
	if err := t.validateClockBoundary(prior, reading); err != nil {
		return switchwire.Envelope{}, err
	}
	if err := t.validateClockBoundary(frame, reading); err != nil {
		return switchwire.Envelope{}, err
	}
	if exact && len(current.Receipt) != 0 {
		return switchwire.ReadEnvelope(bytes.NewReader(current.Receipt))
	}
	if !exact {
		current.AcceptedRequest, current.Receipt = encoded, nil
		if _, err := t.commit(current); err != nil {
			return switchwire.Envelope{}, err
		}
	}
	return t.persistReceipt(current, frame.State, switchwire.PhaseArmed)
}

func (t *Transitions) validateCheckin(request switchwire.Envelope) (switchwire.Frame, []byte, error) {
	frame, err := switchwire.VerifyOwnerRequest(request, t.enrollment)
	if err != nil || frame.Operation != switchwire.OperationCheckin || frame.Phase != switchwire.PhaseArmed || frame.Released {
		return switchwire.Frame{}, nil, errors.New("owner check-in request is invalid")
	}
	encoded, err := switchwire.MarshalEnvelope(request)
	return frame, encoded, err
}

func (t *Transitions) checkinAuthority() (State, switchwire.Frame, error) {
	current, err := t.journal.Snapshot()
	if err != nil {
		return State{}, switchwire.Frame{}, fmt.Errorf("read agent check-in authority: %w", err)
	}
	if current.Phase != switchwire.PhaseArmed {
		return State{}, switchwire.Frame{}, ErrTransitionConflict
	}
	if err := validateArmedState(t, current); err != nil {
		return State{}, switchwire.Frame{}, err
	}
	accepted, err := switchwire.ReadEnvelope(bytes.NewReader(current.AcceptedRequest))
	if err != nil {
		return State{}, switchwire.Frame{}, err
	}
	prior, err := switchwire.DecodeFrame(accepted.Body)
	return current, prior, err
}

func validRenewal(prior, next switchwire.State) bool {
	sequence, deadline := prior.Sequence, prior.Deadline
	prior.Sequence, prior.Deadline = next.Sequence, next.Deadline
	return prior == next && next.Sequence > sequence && next.Deadline >= deadline
}

func (t *Transitions) validateRenewedMaterial(state State, frame switchwire.Frame) error {
	prepare, err := switchwire.ReadEnvelope(bytes.NewReader(state.PrepareRequest))
	if err != nil {
		return fmt.Errorf("decode original prepare request: %w", err)
	}
	initial, err := switchwire.VerifyOwnerRequest(prepare, t.enrollment)
	if err != nil || initial.Operation != switchwire.OperationPrepare || initial.State.Sequence != 1 || !validRenewal(initial.State, frame.State) {
		return ErrTransitionConflict
	}
	return t.validateStoredPrepare(state, initial.State)
}
