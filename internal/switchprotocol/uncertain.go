package switchprotocol

import (
	"bytes"
	"errors"
)

// UncertainModel applies ADR 0012 separately from the preserved B0-only experiments.
// All persistence operations copy values; they establish no disk durability claim.
type UncertainModel struct {
	client   *ClientJournal
	faults   *FaultModel
	canceled bool
	inflight []SignedIntent
}

// NewUncertainModel uses an asserted absolute UTC error bound in both directions.
// Exact elapsed time preserves the conservative UTC lower bound across samples.
func NewUncertainModel(rules Rules, state State, clock ClockReading, utcError uint64) (*UncertainModel, error) {
	if utcError > ^uint64(0)/2 {
		return nil, errors.New("clock error allowance overflows")
	}
	lower, err := subtractSeconds(clock.UTC, utcError)
	if err != nil {
		return nil, err
	}
	clock.UTC = lower
	faults, err := NewFaultModel(rules, state, clock, 2*utcError)
	if err != nil {
		return nil, err
	}
	for i := range faults.agents {
		faults.agents[i].live.utcError = utcError
		faults.agents[i].durable = faults.agents[i].live
		faults.agents[i].initial = faults.agents[i].live
	}
	return &UncertainModel{client: &ClientJournal{confirmed: state}, faults: faults}, nil
}

// Deliver accepts only exact journaled bytes from an authenticated synthetic owner.
// persist and receipt separate accepted memory, durable state, and client evidence.
func (m *UncertainModel) Deliver(agent int, intent SignedIntent, clock ClockReading, persist, receipt bool) error {
	if err := m.authorizeDelivery(intent); err != nil {
		return err
	}
	a, err := m.faults.onlineAgent(agent)
	if err != nil {
		return err
	}
	if receipt && !persist {
		return errors.New("receipt requires persistence")
	}
	if a.live.accepted != intent.State {
		err = a.live.CheckIn(intent.State, clock)
	} else {
		var eligible bool
		eligible, err = a.live.ReleaseEligible(clock)
		if err == nil && eligible {
			err = errors.New("local release boundary reached")
		}
	}
	if persist {
		a.durable = a.live
	}
	if err != nil {
		return err
	}
	if receipt && m.client != nil {
		m.client.receive(m.faults.rules, Receipt{Agent: m.faults.rules.agents[agent], Accepted: intent.State, DurableSequence: intent.State.Sequence, Durable: true})
	}
	return nil
}

// Transmit copies durable intent into the synthetic network before client loss.
func (m *UncertainModel) Transmit(intent SignedIntent) error {
	if m.canceled || m.client == nil || !m.client.matches(intent) {
		return errors.New("intent is unavailable or conflicts")
	}
	m.inflight = append(m.inflight, cloneIntent(intent))
	return nil
}

func (m *UncertainModel) authorizeDelivery(intent SignedIntent) error {
	for _, sent := range m.inflight {
		if sent.State == intent.State && bytes.Equal(sent.Bytes, intent.Bytes) {
			return nil
		}
	}
	if m.client != nil {
		return m.Transmit(intent)
	}
	return errors.New("intent was not transmitted before client loss")
}

// RestartAgent quarantines a known stale snapshot instead of restoring old authority.
// The experiment assumes rollback detection outside this model.
func (m *UncertainModel) RestartAgent(agent int, stale bool) error {
	if agent >= 0 && agent < len(m.faults.agents) && m.faults.agents[agent].durable.sampled {
		return m.restartSampledAgent(agent, stale)
	}
	if err := m.faults.Restart(agent, stale); err != nil {
		return err
	}
	if stale {
		a := &m.faults.agents[agent]
		a.live.unsafe = true
		a.durable = a.live
	}
	return nil
}

// Cancel ends local renewal attempts without recalling any observer copy.
func (m *UncertainModel) Cancel() { m.canceled = true }

// LoseClient removes the owner path permanently from subsequent recovery steps.
func (m *UncertainModel) LoseClient() { m.client = nil }

// Allowance supplies symbolic assumptions for absolute UTC error and publication work.
// Tests instantiate synthetic values; no production value has review acceptance.
type Allowance struct {
	UTCError    uint64
	Publication uint64
}

func (a Allowance) ReleaseBy(bmax int64) (int64, error) {
	if a.UTCError > ^uint64(0)/2 {
		return 0, errors.New("clock error allowance overflows")
	}
	clock, err := addSeconds(bmax, 2*a.UTCError)
	if err != nil {
		return 0, err
	}
	return addSeconds(clock, a.Publication)
}

func subtractSeconds(utc int64, seconds uint64) (int64, error) {
	ordered := uint64(utc) ^ (uint64(1) << 63) // #nosec G115 -- The conversion preserves the signed UTC bit pattern.
	if seconds > ordered {
		return 0, errors.New("UTC lower bound underflows")
	}
	return int64((ordered - seconds) ^ (uint64(1) << 63)), nil // #nosec G115 -- The conversion restores the signed UTC bit pattern.
}
