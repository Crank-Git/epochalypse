package switchprotocol

import "testing"

func TestFreshEpochPublishesWithoutContinuousElapsed(t *testing.T) {
	rules, s := faultScenario(t)
	m, err := NewSampledModel(rules, s.Initial, s.Start, 2)
	requireFaultStep(t, err)
	requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: 105, Elapsed: 5, Safe: true}))
	requireFaultStep(t, m.faults.Crash(0))
	requireFaultStep(t, m.RestartAgent(0, false))
	// A new process samples real UTC 114 with a reset elapsed epoch.
	requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: 112, Elapsed: 0, Safe: true}))
	if m.faults.observer.public != 1 {
		t.Fatal("fresh epoch fails to publish by the conservative boundary")
	}
}

func TestFreshSamplesIgnoreElapsedRate(t *testing.T) {
	rules, s := faultScenario(t)
	m, err := NewSampledModel(rules, s.Initial, s.Start, 2)
	requireFaultStep(t, err)
	// Real UTC is 109; an arbitrary elapsed value supplies no release authority.
	requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: 111, Elapsed: 10000, Safe: true}))
	if m.faults.observer.public != 0 {
		t.Fatal("elapsed rate permits early publication")
	}
	requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: 112, Elapsed: 0, Safe: true}))
	if m.faults.observer.public != 1 {
		t.Fatal("fresh lower bound fails to publish")
	}
}
