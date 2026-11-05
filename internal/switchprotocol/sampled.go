package switchprotocol

import "errors"

// NewSampledModel returns a UTC-only model without an elapsed-clock assumption.
// Safe asserts fresh absolute accuracy and validated durable state at each decision.
// The caller validates restored state before it supplies the first trusted sample.
func NewSampledModel(rules Rules, state State, clock ClockReading, utcError uint64) (*UncertainModel, error) {
	m, err := NewUncertainModel(rules, state, clock, utcError)
	if err != nil {
		return nil, err
	}
	for i := range m.faults.agents {
		a := &m.faults.agents[i]
		a.live.sampled = true
		a.durable = a.live
		a.initial = a.live
	}
	return m, nil
}

func (a *ModelAgent) sampledEligibility(reading ClockReading) (bool, error) {
	if a.unsafe || !reading.Safe {
		a.unsafe = true
		return false, errors.New("fresh clock assessment is unavailable or unsafe")
	}
	lower, err := subtractSeconds(reading.UTC, a.utcError)
	if err != nil {
		a.unsafe = true
		return false, err
	}
	// A previous sample supplies no time authority in a new clock epoch.
	a.utc = lower
	boundary, _ := releaseBoundary(a.accepted)
	a.released = a.released || lower >= boundary
	return a.released, nil
}

func (m *UncertainModel) restartSampledAgent(agent int, stale bool) error {
	// Quarantine preserves known authority instead of replacing it with a stale backup.
	if err := m.faults.Restart(agent, false); err != nil {
		return err
	}
	a := &m.faults.agents[agent]
	if stale {
		a.live.unsafe = true
		a.durable = a.live
	}
	return nil
}
