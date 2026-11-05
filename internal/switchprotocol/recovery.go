package switchprotocol

import "errors"

// RecoveryScenario supplies independent agent copies and public evidence after main-server loss.
// Retained contains authenticated public shares from earlier history for the same content.
type RecoveryScenario struct {
	FaultScenario
	Ciphertext [3]bool
	Retained   []PublicShare
	Later      ClockReading
}

type RecoveryEvidence struct {
	PublicShares        uint8
	CiphertextAvailable bool
	ContentAvailable    bool
}

type RecoveryResult struct {
	Schedule  uint8
	Outage    int
	Confirmed State
	Accepted  [3]State
	Observed  RecoveryEvidence
	Later     RecoveryEvidence
}

// ExploreRecoveryFaults repeats each renewal schedule with each one-agent outage.
// Content availability means a public threshold and a surviving ciphertext copy, not actual decryption.
func ExploreRecoveryFaults(rules Rules, scenario RecoveryScenario) ([]RecoveryResult, error) {
	results := make([]RecoveryResult, 0, 192)
	for schedule := uint8(0); schedule < 64; schedule++ {
		for outage := 0; outage < 3; outage++ {
			result, err := exploreRecovery(rules, scenario, schedule, outage)
			if err != nil {
				return nil, err
			}
			results = append(results, result)
		}
	}
	return results, nil
}

func exploreRecovery(rules Rules, s RecoveryScenario, schedule uint8, outage int) (RecoveryResult, error) {
	m, err := recoveryModel(rules, s, schedule)
	if err != nil {
		return RecoveryResult{}, err
	}
	if err := m.Crash(outage); err != nil {
		return RecoveryResult{}, err
	}
	result := RecoveryResult{Schedule: schedule, Outage: outage, Confirmed: m.Confirmed(), Accepted: m.Accepted()}
	result.Observed, err = recoveryEvidence(m, s.Ciphertext, s.Observe)
	if err != nil {
		return RecoveryResult{}, err
	}
	result.Later, err = recoveryEvidence(m, s.Ciphertext, s.Later)
	if err != nil {
		return RecoveryResult{}, err
	}
	return result, nil
}

func recoveryModel(rules Rules, s RecoveryScenario, schedule uint8) (*FaultModel, error) {
	m, err := NewFaultModel(rules, s.Initial, s.Start, s.Tolerance)
	if err != nil {
		return nil, err
	}
	for _, share := range s.Retained {
		if !m.observer.Observe(share) {
			return nil, errors.New("invalid retained public share")
		}
	}
	for agent := range m.agents {
		stage := (schedule >> (2 * agent)) & 3
		if stage != 0 {
			if err := m.CheckIn(agent, s.Next, s.Renew, stage >= 2, stage == 3); err != nil {
				return nil, err
			}
		}
		// Resume presumed durable state before the permanent outage begins.
		m.agents[agent].online = false
		m.agents[agent].resume(false)
	}
	return m, nil
}

func recoveryEvidence(m *FaultModel, ciphertext [3]bool, clock ClockReading) (RecoveryEvidence, error) {
	result := RecoveryEvidence{}
	for agent, state := range m.agents {
		if !state.online {
			continue
		}
		if err := m.Publish(agent, clock); err != nil {
			return RecoveryEvidence{}, err
		}
		result.CiphertextAvailable = result.CiphertextAvailable || ciphertext[agent]
	}
	result.PublicShares = m.observer.public
	result.ContentAvailable = result.CiphertextAvailable && m.ReleaseAvailable()
	return result, nil
}
