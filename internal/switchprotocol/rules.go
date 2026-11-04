// Package switchprotocol models candidate rules for protocol review with synthetic data.
package switchprotocol

import "errors"

type AgentID string

type State struct {
	SwitchID        string
	ProtocolVersion uint32
	Sequence        uint64
	Deadline        int64
	Grace           uint64
	ContentDigest   [32]byte
}

type Receipt struct {
	Agent           AgentID
	Accepted        State
	DurableSequence uint64
	Durable         bool
}

type PublicShare struct {
	Agent           AgentID
	SwitchID        string
	ProtocolVersion uint32
	ContentDigest   [32]byte
}

type Rules struct {
	agents [3]AgentID
}

func NewRules(agents [3]AgentID) (Rules, error) {
	for i, agent := range agents {
		if agent == "" {
			return Rules{}, errors.New("agent identity is empty")
		}
		for _, previous := range agents[:i] {
			if agent == previous {
				return Rules{}, errors.New("agent identities are not distinct")
			}
		}
	}
	return Rules{agents: agents}, nil
}

// RenewalConfirmed requires evidence for the exact requested state from every agent.
// The caller supplies authenticated receipts after durable storage in the real protocol.
func (r Rules) RenewalConfirmed(state State, receipts []Receipt) bool {
	var confirmed uint8
	for _, receipt := range receipts {
		if receipt.Accepted == state && receipt.Durable && receipt.DurableSequence == state.Sequence {
			confirmed |= r.agentBit(receipt.Agent)
		}
	}
	return confirmed == 7
}

func (r Rules) agentBit(agent AgentID) uint8 {
	for i, known := range r.agents {
		if agent != "" && agent == known {
			return 1 << i
		}
	}
	return 0
}

type Observer struct {
	rules  Rules
	state  State
	public uint8
}

func (r Rules) NewObserver(state State) Observer { return Observer{rules: r, state: state} }

// Observe retains a public share without a deadline or sequence filter.
// A renewal cannot recall a public copy of the same content key share.
func (o *Observer) Observe(share PublicShare) bool {
	if share.SwitchID != o.state.SwitchID || share.ProtocolVersion != o.state.ProtocolVersion || share.ContentDigest != o.state.ContentDigest {
		return false
	}
	bit := o.rules.agentBit(share.Agent)
	o.public |= bit
	return bit != 0
}

// ReleaseAvailable reports whether two distinct agents have disclosed their shares.
// The model assumes valid shares; it does not reconstruct a content key.
func (o Observer) ReleaseAvailable() bool {
	return o.public != 0 && o.public&(o.public-1) != 0
}
