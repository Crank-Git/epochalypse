package agent

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

// ServiceConfig pins the existing authority and the clock mode for one agent.
type ServiceConfig struct {
	Authority             Config
	Transition            TransitionConfig
	VerifiedClock         *ChronyClock
	LabOnlySyntheticClock bool
}

// Service exposes one validated agent transition handler.
type Service struct {
	journal     *Journal
	transitions *Transitions
}

// OpenService validates authority and enrollment before it returns readiness.
func OpenService(config ServiceConfig) (*Service, error) {
	if config.LabOnlySyntheticClock == (config.VerifiedClock != nil) {
		return nil, errors.New("configure exactly one clock mode")
	}
	clock := func() (Reading, error) { return Reading{}, ErrClock }
	if config.LabOnlySyntheticClock {
		clock = config.Transition.ReadClock
	}
	if clock == nil {
		return nil, ErrClock
	}
	if config.VerifiedClock != nil {
		clock = config.VerifiedClock.Read
	}
	journal, err := Open(config.Authority)
	if err != nil {
		return nil, fmt.Errorf("open agent authority: %w", err)
	}
	config.Transition.Journal = journal
	config.Transition.ReadClock = clock
	transitions, err := NewTransitions(config.Transition)
	if err != nil {
		_ = journal.Close()
		return nil, fmt.Errorf("validate agent enrollment: %w", err)
	}
	if err := validateServiceState(transitions, journal); err != nil {
		_ = journal.Close()
		return nil, err
	}
	if _, err := clock(); err != nil {
		err = errors.Join(err, latchStartupClockFailure(transitions, journal))
		_ = journal.Close()
		return nil, fmt.Errorf("assess agent startup clock: %w", err)
	}
	return &Service{journal: journal, transitions: transitions}, nil
}

func latchStartupClockFailure(transitions *Transitions, journal *Journal) error {
	state, err := journal.Snapshot()
	if err != nil {
		return fmt.Errorf("read authority after unsafe startup clock: %w", err)
	}
	if state.Phase == 0 {
		return nil
	}
	state.UnsafeClock = true
	if _, err := transitions.commit(state); err != nil {
		return fmt.Errorf("persist unsafe startup clock: %w", err)
	}
	return nil
}

func validateServiceState(transitions *Transitions, journal *Journal) error {
	state, err := journal.Snapshot()
	if err != nil {
		return fmt.Errorf("read agent authority before readiness: %w", err)
	}
	if state.PossibleDisclosure || state.UnsafeClock {
		return ErrQuarantined
	}
	if state.Phase == 0 {
		if state.Generation != 0 || !emptyAgentState(state) {
			return ErrQuarantined
		}
		return nil
	}
	if state.Phase == switchwire.PhasePrepared {
		return validatePreparedState(transitions, state)
	}
	if state.Phase == switchwire.PhaseArmed {
		return validateArmedState(transitions, state)
	}
	return ErrQuarantined
}

func validatePreparedState(transitions *Transitions, state State) error {
	if len(state.Manifest) != 0 || len(state.ReleaseEnvelope) != 0 {
		return ErrQuarantined
	}
	request, err := switchwire.ReadEnvelope(bytes.NewReader(state.AcceptedRequest))
	if err != nil {
		return fmt.Errorf("decode durable prepare request: %w", err)
	}
	frame, err := switchwire.VerifyOwnerRequest(request, transitions.enrollment)
	if err != nil || frame.Operation != switchwire.OperationPrepare {
		return ErrQuarantined
	}
	if err := transitions.validateStoredPrepare(state, frame.State); err != nil {
		return fmt.Errorf("validate durable prepared material: %w", err)
	}
	if len(state.Receipt) == 0 {
		return nil
	}
	return transitions.validateReceipt(state.Receipt, frame.State, switchwire.PhasePrepared)
}

func validateArmedState(transitions *Transitions, state State) error {
	if len(state.Manifest) != 0 || len(state.ReleaseEnvelope) != 0 {
		return ErrQuarantined
	}
	request, err := switchwire.ReadEnvelope(bytes.NewReader(state.AcceptedRequest))
	if err != nil {
		return fmt.Errorf("decode durable arm request: %w", err)
	}
	frame, err := switchwire.VerifyOwnerRequest(request, transitions.enrollment)
	if err != nil || frame.Operation != switchwire.OperationArm {
		return ErrQuarantined
	}
	if err := transitions.validateStoredPrepare(state, frame.State); err != nil {
		return fmt.Errorf("validate durable prepared material: %w", err)
	}
	if err := transitions.validateAcceptedArm(state.AcceptedRequest, frame.State); err != nil {
		return fmt.Errorf("validate durable arm request: %w", err)
	}
	if len(state.Receipt) == 0 {
		return nil
	}
	return transitions.validateReceipt(state.Receipt, frame.State, switchwire.PhaseArmed)
}

func emptyAgentState(state State) bool {
	return len(state.AcceptedRequest) == 0 && len(state.PrepareRequest) == 0 && len(state.Enrollment) == 0 &&
		len(state.Ciphertext) == 0 && len(state.Manifest) == 0 && len(state.EncryptedShare) == 0 &&
		len(state.Receipt) == 0 && len(state.ReleaseEnvelope) == 0
}

// Handler returns the bounded transition endpoints.
func (s *Service) Handler() http.Handler { return s.transitions.Handler() }

// Close releases the exclusive authority lock.
func (s *Service) Close() error { return s.journal.Close() }
