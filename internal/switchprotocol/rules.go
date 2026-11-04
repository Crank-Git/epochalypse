// Package switchprotocol models candidate rules for protocol review with synthetic data.
package switchprotocol

import "errors"

type AgentID string

type State struct {
	SwitchID string
	ProtocolVersion uint32
	Sequence uint64
	Deadline int64
	Grace uint64
	ContentDigest [32]byte
}

type Receipt struct {
	Agent AgentID
	Accepted State
	DurableSequence uint64
	Durable bool
}

type PublicShare struct {
	Agent AgentID
	SwitchID string
	ProtocolVersion uint32
	ContentDigest [32]byte
}

type Rules struct {
	agents [3]AgentID
}

func NewRules(agents [3]AgentID) (Rules, error) {
	for i, agent := range agents {
		if agent == "" { return Rules{}, errors.New("agent identity is empty") }
		for _, previous := range agents[:i] {
			if agent == previous { return Rules{}, errors.New("agent identities are not distinct") }
		}
	}
	return Rules{agents: agents}, nil
}

func (r Rules) RenewalConfirmed(state State, receipts []Receipt) bool { return false }

type Observer struct {
	rules Rules
	state State
	public uint8
}

func (r Rules) NewObserver(state State) Observer { return Observer{rules:r, state:state} }
func (o *Observer) Observe(share PublicShare) bool { return false }
func (o Observer) ReleaseAvailable() bool { return false }
