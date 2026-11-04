package switchprotocol

import "errors"

// FaultModel separates owner evidence from local and presumed durable agent state.
type FaultModel struct {
	rules    Rules
	owner    State
	agents   [3]faultAgent
	observer Observer
	receipts []Receipt
}

type faultAgent struct {
	live    ModelAgent
	durable ModelAgent
	initial ModelAgent
	online  bool
}

func NewFaultModel(rules Rules, state State, clock ClockReading, tolerance uint64) (*FaultModel, error) {
	if _, err := NewRules(rules.agents); err != nil {
		return nil, err
	}
	a, err := NewModelAgent(state, clock, tolerance)
	if err != nil {
		return nil, err
	}
	m := &FaultModel{rules: rules, owner: state, observer: rules.NewObserver(state)}
	for i := range m.agents {
		m.agents[i] = faultAgent{live: *a, durable: *a, initial: *a, online: true}
	}
	return m, nil
}

// CheckIn optionally persists the complete model before it delivers a synthetic receipt.
// A receipt cannot attest to volatile state. Lost receipts leave owner evidence unchanged.
func (m *FaultModel) CheckIn(agent int, next State, clock ClockReading, persist, deliver bool) error {
	a, err := m.onlineAgent(agent)
	if err != nil {
		return err
	}
	if deliver && !persist {
		return errors.New("receipt requires persistence")
	}
	err = a.live.CheckIn(next, clock)
	if persist {
		a.durable = a.live
	}
	if err != nil {
		return err
	}
	if deliver {
		m.receipts = append(m.receipts, Receipt{Agent: m.rules.agents[agent], Accepted: next, DurableSequence: next.Sequence, Durable: true})
		if m.rules.RenewalConfirmed(next, m.receipts) {
			m.owner = next
		}
	}
	return nil
}

// Crash removes availability without changing the presumed durable snapshot.
func (m *FaultModel) Crash(agent int) error {
	a, err := m.onlineAgent(agent)
	if err != nil {
		return err
	}
	a.online = false
	return nil
}

// Restart resumes a complete value snapshot, including both latches and clock history.
// rollback deliberately substitutes the initial snapshot to expose an undetected stale backup.
// Elapsed samples continue on the experiment timeline, not a real process clock.
func (m *FaultModel) Restart(agent int, rollback bool) error {
	if agent < 0 || agent >= len(m.agents) {
		return errors.New("unknown model agent")
	}
	a := &m.agents[agent]
	if a.online {
		return errors.New("model agent is already online")
	}
	a.resume(rollback)
	return nil
}

func (a *faultAgent) resume(rollback bool) {
	if rollback {
		a.durable = a.initial
	}
	a.live = a.durable
	a.online = true
}

// Publish retains a share only after local eligibility succeeds.
// The experiment presumes durable latches before public disclosure.
func (m *FaultModel) Publish(agent int, clock ClockReading) error {
	a, err := m.onlineAgent(agent)
	if err != nil {
		return err
	}
	eligible, err := a.live.ReleaseEligible(clock)
	a.durable = a.live
	if err != nil {
		return err
	}
	if eligible {
		s := a.live.Accepted()
		m.observer.Observe(PublicShare{Agent: m.rules.agents[agent], SwitchID: s.SwitchID, ProtocolVersion: s.ProtocolVersion, ContentDigest: s.ContentDigest})
	}
	return nil
}

func (m *FaultModel) onlineAgent(agent int) (*faultAgent, error) {
	if agent < 0 || agent >= len(m.agents) {
		return nil, errors.New("unknown model agent")
	}
	a := &m.agents[agent]
	if !a.online {
		return nil, errors.New("model agent is offline")
	}
	return a, nil
}

func (m *FaultModel) Confirmed() State { return m.owner }
func (m *FaultModel) Accepted() [3]State {
	var states [3]State
	for i := range m.agents {
		states[i] = m.agents[i].live.Accepted()
	}
	return states
}
func (m *FaultModel) ReleaseAvailable() bool { return m.observer.ReleaseAvailable() }

// FaultScenario bounds one renewal experiment with exact synthetic clock samples.
type FaultScenario struct {
	Initial   State
	Next      State
	Start     ClockReading
	Renew     ClockReading
	Observe   ClockReading
	Tolerance uint64
}

type FaultResult struct {
	Schedule         uint8
	Updated          uint8
	Persisted        uint8
	Receipts         uint8
	Confirmed        State
	Accepted         [3]State
	ReleaseAvailable bool
}

// ExploreRenewalFaults enumerates all 64 combinations of four local fault stages.
// Each agent misses the request, loses volatile state, loses its receipt, or delivers its receipt.
// Every agent then crashes, resumes its presumed durable snapshot, and attempts publication.
func ExploreRenewalFaults(rules Rules, scenario FaultScenario) ([]FaultResult, error) {
	results := make([]FaultResult, 0, 64)
	for schedule := uint8(0); schedule < 64; schedule++ {
		result, err := exploreRenewal(rules, scenario, schedule)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func exploreRenewal(rules Rules, s FaultScenario, schedule uint8) (FaultResult, error) {
	m, err := NewFaultModel(rules, s.Initial, s.Start, s.Tolerance)
	if err != nil {
		return FaultResult{}, err
	}
	result := FaultResult{Schedule: schedule}
	for agent := range m.agents {
		stage := (schedule >> (2 * agent)) & 3
		if stage != 0 {
			if err := m.CheckIn(agent, s.Next, s.Renew, stage >= 2, stage == 3); err != nil {
				return FaultResult{}, err
			}
			result.Updated |= 1 << agent
		}
		if stage >= 2 {
			result.Persisted |= 1 << agent
		}
		if stage == 3 {
			result.Receipts |= 1 << agent
		}
		m.agents[agent].online = false
		m.agents[agent].resume(false)
		if err := m.Publish(agent, s.Observe); err != nil {
			return FaultResult{}, err
		}
	}
	result.Confirmed = m.Confirmed()
	result.Accepted = m.Accepted()
	result.ReleaseAvailable = m.ReleaseAvailable()
	return result, nil
}
