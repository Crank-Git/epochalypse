package switchprotocol

import "testing"

// This probe uses the shared constructor interface to test both clock policies.
func TestConservativePolicyRejectsFastClockEarlyPublication(t *testing.T) {
	rules, err := NewRules([3]AgentID{"zero", "one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	state := State{SwitchID: "synthetic", ProtocolVersion: 1, Sequence: 1, Deadline: 100, Grace: 10}
	m, err := NewUncertainModel(rules, state, ClockReading{UTC: 100, Safe: true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.faults.Publish(0, ClockReading{UTC: 110, Elapsed: 9, Safe: true}); err != nil {
		t.Fatal(err)
	}
	if m.faults.observer.public != 0 {
		t.Fatal("fast clock publishes at real UTC 109 before B0 110")
	}
}
