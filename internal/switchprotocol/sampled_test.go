package switchprotocol

import (
	"math"
	"testing"
)

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

func TestSampledEpochsReleaseEveryTwoIntentOutageSchedule(t *testing.T) {
	rules, s := faultScenario(t)
	count := 0
	for first := uint8(0); first < 64; first++ {
		for second := uint8(0); second < 64; second++ {
			for outage := 0; outage < 3; outage++ {
				checkSampledRecovery(t, rules, s, first, second, outage)
				count++
			}
		}
	}
	if count != 12288 {
		t.Fatal("omits an issued-intent outage schedule")
	}
}

func checkSampledRecovery(t *testing.T, rules Rules, s FaultScenario, first, second uint8, outage int) {
	t.Helper()
	m, err := NewSampledModel(rules, s.Initial, ClockReading{UTC: 102, Safe: true}, 2)
	requireFaultStep(t, err)
	one := SignedIntent{State: s.Next, Bytes: []byte("first synthetic intent")}
	requireFaultStep(t, m.client.Issue(one, true))
	applyUncertainSchedule(t, m, one, first, ClockReading{UTC: 99, Elapsed: 10000, Safe: true})
	two := cloneIntent(one)
	two.State.Sequence++
	two.State.Deadline = 300
	two.Bytes = []byte("second synthetic intent")
	requireFaultStep(t, m.client.Issue(two, true))
	applyUncertainSchedule(t, m, two, second, ClockReading{UTC: 104, Elapsed: 0, Safe: true})
	if bounds := m.client.Bounds(); bounds.Bmax != 310 || (bounds.Confirmed && second != 63) {
		t.Fatalf("unsupported client outcome: %+v", bounds)
	}
	m.LoseClient()
	requireFaultStep(t, m.faults.Crash(outage))
	for agent := range m.faults.agents {
		if agent == outage {
			continue
		}
		// Independent offsets change across samples and reset elapsed epochs.
		requireFaultStep(t, m.faults.Publish(agent, ClockReading{UTC: 111, Elapsed: math.MaxUint64, Safe: true}))
	}
	if m.faults.observer.public != 0 {
		t.Fatal("publishes before real UTC 110")
	}
	// The slowest allowed sample follows the full five-second sample gap.
	evidence, err := recoveryEvidence(m.faults, [3]bool{true, true, true}, ClockReading{UTC: 317, Safe: true})
	requireFaultStep(t, err)
	if !evidence.ContentAvailable {
		t.Fatal("fresh epochs exceed Bmax plus two epsilon plus the sample gap")
	}
}

func TestSampledRestartPreservesAuthorityAndObserverHistory(t *testing.T) {
	rules, s := faultScenario(t)
	m, err := NewSampledModel(rules, s.Initial, s.Start, 2)
	requireFaultStep(t, err)
	intent := SignedIntent{State: s.Next, Bytes: []byte("synthetic intent")}
	requireFaultStep(t, m.client.Issue(intent, true))
	requireFaultStep(t, m.Deliver(0, intent, s.Renew, true, false))
	requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: 212, Safe: true}))
	requireFaultStep(t, m.faults.Crash(0))
	requireFaultStep(t, m.RestartAgent(0, true))
	a := m.faults.agents[0].durable
	if a.accepted != intent.State || !a.released || !a.unsafe || m.faults.observer.public != 1 {
		t.Fatal("quarantine erases accepted state or irreversible history")
	}
	requireFaultStep(t, m.faults.Crash(0))
	requireFaultStep(t, m.RestartAgent(0, false))
	if err := m.faults.Publish(0, ClockReading{UTC: 300, Safe: true}); err == nil {
		t.Fatal("restart forgets unsafe state")
	}
	if err := m.Deliver(0, intent, ClockReading{UTC: 300, Safe: true}, true, true); err == nil {
		t.Fatal("unsafe released agent renews")
	}
	if m.client.Bounds().Confirmed {
		t.Fatal("quarantine issues unsupported confirmation")
	}
}

func TestSampledRestartRequiresFreshAssessment(t *testing.T) {
	for _, stale := range []bool{false, true} {
		rules, s := faultScenario(t)
		m, err := NewSampledModel(rules, s.Initial, s.Start, 2)
		requireFaultStep(t, err)
		requireFaultStep(t, m.faults.Crash(0))
		requireFaultStep(t, m.RestartAgent(0, stale))
		if err := m.faults.Publish(0, ClockReading{UTC: 1000}); err == nil {
			t.Fatal("missing or expired assessment permits publication")
		}
		requireFaultStep(t, m.faults.Crash(0))
		requireFaultStep(t, m.RestartAgent(0, false))
		if err := m.faults.Publish(0, ClockReading{UTC: 1000, Safe: true}); err == nil {
			t.Fatal("fresh assessment clears durable unsafe latch")
		}
		if m.faults.observer.public != 0 {
			t.Fatal("unready clock creates observer evidence")
		}
	}
}

func TestSampledArithmeticAndRestartFailuresPreserveSafety(t *testing.T) {
	rules, s := faultScenario(t)
	if _, err := NewSampledModel(rules, s.Initial, ClockReading{}, 2); err == nil {
		t.Fatal("unsafe constructor succeeds")
	}
	m, err := NewSampledModel(rules, s.Initial, s.Start, 2)
	requireFaultStep(t, err)
	if err := m.RestartAgent(0, false); err == nil {
		t.Fatal("online agent restarts")
	}
	if err := m.RestartAgent(3, false); err == nil {
		t.Fatal("unknown agent restarts")
	}
	if err := m.faults.Publish(0, ClockReading{UTC: math.MinInt64, Safe: true}); err == nil {
		t.Fatal("unrepresentable lower bound succeeds")
	}
	if !m.faults.agents[0].durable.unsafe || m.faults.observer.public != 0 {
		t.Fatal("arithmetic failure loses quarantine")
	}
}

func TestCandidateProfileBoundsSampleGapAndRecoveryWork(t *testing.T) {
	rules, s := faultScenario(t)
	by, err := (Allowance{UTCError: 2, Publication: 30}).ReleaseBy(110)
	requireFaultStep(t, err)
	if by != 144 {
		t.Fatal("candidate allowance differs from 34 seconds")
	}
	for offset := int64(-2); offset <= 2; offset++ {
		for gap := int64(0); gap <= 5; gap++ {
			for work := int64(0); work <= 25; work++ {
				m, err := NewSampledModel(rules, s.Initial, s.Start, 2)
				requireFaultStep(t, err)
				requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: 109 + offset, Safe: true}))
				if m.faults.observer.public != 0 {
					t.Fatal("candidate permits early disclosure")
				}
				sampleTime := 114 + gap
				requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: sampleTime + offset, Safe: true}))
				if m.faults.observer.public != 1 || sampleTime+work > by {
					t.Fatal("candidate exceeds its sample or recovery budget")
				}
			}
		}
	}
}

func TestSampledRestartCountsEveryRetainedShareSubset(t *testing.T) {
	for mask := uint8(0); mask < 8; mask++ {
		rules, s := faultScenario(t)
		m, err := NewSampledModel(rules, s.Initial, s.Start, 2)
		requireFaultStep(t, err)
		for agent := range m.faults.agents {
			if mask&(1<<agent) != 0 {
				requireFaultStep(t, m.faults.Publish(agent, ClockReading{UTC: 112, Safe: true}))
			}
			requireFaultStep(t, m.faults.Crash(agent))
			requireFaultStep(t, m.RestartAgent(agent, false))
		}
		m.Cancel()
		if m.faults.observer.public != mask {
			t.Fatal("restart or cancellation forgets retained shares")
		}
	}
}
