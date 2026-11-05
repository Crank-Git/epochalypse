package switchprotocol

import (
	"math"
	"testing"
)

func uncertainFixture(t *testing.T) (*UncertainModel, FaultScenario, SignedIntent) {
	t.Helper()
	rules, s := faultScenario(t)
	m, err := NewUncertainModel(rules, s.Initial, s.Start, 0)
	requireFaultStep(t, err)
	intent := SignedIntent{State: s.Next, Bytes: []byte("synthetic signed renewal")}
	requireFaultStep(t, m.client.Issue(intent, true))
	return m, s, intent
}

func TestLostFinalReceiptsReportUncertaintyAfterCompleteAcceptance(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		m, s, intent := uncertainFixture(t)
		for agent := 0; agent < 3; agent++ {
			requireFaultStep(t, m.Deliver(agent, intent, s.Renew, true, mask&(1<<agent) != 0))
		}
		bounds := m.client.Bounds()
		if mask == 7 {
			if bounds != (Uncertainty{B0: 210, Bmax: 210, Confirmed: true}) {
				t.Fatalf("wrong confirmation: %+v", bounds)
			}
		} else if bounds != (Uncertainty{B0: 110, Bmax: 210}) {
			t.Fatalf("receipt loss hides uncertainty: %+v", bounds)
		}
		if m.faults.Accepted() != [3]State{s.Next, s.Next, s.Next} {
			t.Fatal("confuses receipt loss with rejected state")
		}
	}
}

func TestRetriesPreserveSignedDeadlineAcrossArrivalAndRestart(t *testing.T) {
	m, s, intent := uncertainFixture(t)
	requireFaultStep(t, m.Deliver(0, intent, s.Renew, false, false))
	requireFaultStep(t, m.faults.Crash(0))
	requireFaultStep(t, m.RestartAgent(0, false))
	m.client = m.client.Restart()
	retry, err := m.client.Retry(intent.State.Sequence)
	requireFaultStep(t, err)
	late := ClockReading{UTC: 109, Elapsed: 9, Safe: true}
	requireFaultStep(t, m.Deliver(0, retry, late, true, false))
	requireFaultStep(t, m.Deliver(0, retry, ClockReading{UTC: 150, Elapsed: 50, Safe: true}, true, true))
	if m.faults.Accepted()[0].Deadline != 200 {
		t.Fatal("arrival extends signed deadline")
	}
	requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: 209, Elapsed: 109, Safe: true}))
	if m.faults.observer.public != 0 {
		t.Fatal("publishes before signed boundary")
	}
	requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: 210, Elapsed: 110, Safe: true}))
	if m.faults.observer.public != 1 {
		t.Fatal("retry postpones release")
	}
	if err := m.Deliver(0, retry, ClockReading{UTC: 211, Elapsed: 111, Safe: true}, true, true); err == nil {
		t.Fatal("released agent accepts retry")
	}
	if err := m.Deliver(1, retry, ClockReading{UTC: 110, Elapsed: 10, Safe: true}, true, true); err == nil {
		t.Fatal("late delivery reopens old boundary")
	}
}

func applyUncertainSchedule(t *testing.T, m *UncertainModel, intent SignedIntent, schedule uint8, clock ClockReading) {
	t.Helper()
	for agent := 0; agent < 3; agent++ {
		stage := (schedule >> (2 * agent)) & 3
		if stage != 0 {
			requireFaultStep(t, m.Deliver(agent, intent, clock, stage >= 2, stage == 3))
		}
		requireFaultStep(t, m.faults.Crash(agent))
		requireFaultStep(t, m.RestartAgent(agent, false))
	}
}

func TestReleasesEveryTwoIntentScheduleByBmaxPlusAllowance(t *testing.T) {
	rules, s := faultScenario(t)
	count := 0
	for first := uint8(0); first < 64; first++ {
		for second := uint8(0); second < 64; second++ {
			for outage := 0; outage < 3; outage++ {
				count += checkSymmetricOffsets(t, rules, s, first, second, outage)
			}
		}
	}
	if count != 110592 {
		t.Fatal("omits a two-intent outage schedule")
	}
}

func checkSymmetricOffsets(t *testing.T, rules Rules, s FaultScenario, first, second uint8, outage int) int {
	t.Helper()
	count := 0
	for epsilon := int64(0); epsilon <= 2; epsilon++ {
		for offset := -epsilon; offset <= epsilon; offset++ {
			checkTwoIntentRecovery(t, rules, s, first, second, outage, uint64(epsilon), offset) // #nosec G115 -- Epsilon is between zero and two.
			count++
		}
	}
	return count
}

func checkTwoIntentRecovery(t *testing.T, rules Rules, s FaultScenario, first, second uint8, outage int, epsilon uint64, offset int64) {
	t.Helper()
	// Each synthetic value instantiates symbolic assumptions; none defines production E.
	allowance := Allowance{UTCError: epsilon, Publication: uint64(second % 3)}
	start := s.Start
	start.UTC += offset
	m, err := NewUncertainModel(rules, s.Initial, start, allowance.UTCError)
	requireFaultStep(t, err)
	one := SignedIntent{State: s.Next, Bytes: []byte("first signed intent")}
	requireFaultStep(t, m.client.Issue(one, true))
	applyUncertainSchedule(t, m, one, first, ClockReading{UTC: start.UTC + 1, Elapsed: 1, Safe: true})
	two := SignedIntent{State: s.Next, Bytes: []byte("second signed intent")}
	two.State.Sequence++
	two.State.Deadline = 300
	requireFaultStep(t, m.client.Issue(two, true))
	applyUncertainSchedule(t, m, two, second, ClockReading{UTC: start.UTC + 2, Elapsed: 2, Safe: true})
	bounds := m.client.Bounds()
	if bounds.Bmax != 310 {
		t.Fatal("loses latest issued intent")
	}
	by, err := allowance.ReleaseBy(bounds.Bmax)
	requireFaultStep(t, err)
	m.LoseClient()
	requireFaultStep(t, m.faults.Crash(outage))
	before := ClockReading{UTC: 109 + offset, Elapsed: 9, Safe: true}
	early, err := recoveryEvidence(m.faults, [3]bool{true, true, true}, before)
	requireFaultStep(t, err)
	if early.PublicShares != 0 {
		t.Fatal("publishes before initial B0")
	}
	// Publication completes within the assumed work allowance after local eligibility.
	clock := ClockReading{UTC: by + offset, Elapsed: uint64(by - 100), Safe: true} // #nosec G115 -- This experiment uses positive times below 1000.
	evidence, err := recoveryEvidence(m.faults, [3]bool{true, true, true}, clock)
	requireFaultStep(t, err)
	if !evidence.ContentAvailable || evidence.PublicShares != 7&^(1<<outage) {
		t.Fatalf("schedule %d/%d outage %d misses Bmax+E", first, second, outage)
	}
}

func TestConservativePolicyPreservesLowerBoundAcrossClockSteps(t *testing.T) {
	for _, offsets := range [][3]int64{{-2, 0, 2}, {2, 0, -2}, {0, -2, 2}} {
		for outage := 0; outage < 3; outage++ {
			checkClockSteps(t, offsets, outage)
		}
	}
}

func checkClockSteps(t *testing.T, offsets [3]int64, outage int) {
	t.Helper()
	rules, s := faultScenario(t)
	s.Start.UTC += offsets[0]
	m, err := NewUncertainModel(rules, s.Initial, s.Start, 2)
	requireFaultStep(t, err)
	m.LoseClient()
	requireFaultStep(t, m.faults.Crash(outage))
	for index, realUTC := range []int64{109, 110, 113, 114} {
		offset := offsets[min(index, 2)]
		evidence, err := recoveryEvidence(m.faults, [3]bool{true, true, true}, ClockReading{UTC: realUTC + offset, Elapsed: uint64(realUTC - 100), Safe: true}) // #nosec G115 -- These synthetic elapsed values are positive.
		requireFaultStep(t, err)
		if realUTC < 110 && evidence.PublicShares != 0 {
			t.Fatal("clock step publishes before B0")
		}
		if realUTC == 114 && !evidence.ContentAvailable {
			t.Fatal("clock step misses Bmax plus twice the error")
		}
	}
}

func TestConservativePolicyRejectsUnrepresentableBounds(t *testing.T) {
	rules, s := faultScenario(t)
	if _, err := NewUncertainModel(rules, s.Initial, s.Start, math.MaxUint64); err == nil {
		t.Fatal("accepts an unrepresentable initial clock bound")
	}
	if _, err := NewUncertainModel(rules, s.Initial, ClockReading{UTC: math.MinInt64, Safe: true}, 1); err == nil {
		t.Fatal("accepts initial lower bound underflow")
	}
	if _, err := (Allowance{UTCError: math.MaxUint64}).ReleaseBy(110); err == nil {
		t.Fatal("overflows twice the UTC error")
	}
	m, err := NewUncertainModel(rules, s.Initial, s.Start, 1)
	requireFaultStep(t, err)
	if err := m.faults.Publish(0, ClockReading{UTC: math.MinInt64, Elapsed: 1, Safe: true}); err == nil {
		t.Fatal("accepts lower bound underflow")
	}
	if err := m.faults.Publish(0, ClockReading{UTC: 101, Elapsed: 1, Safe: true}); err == nil {
		t.Fatal("underflow clears unsafe latch")
	}
}

func TestConservativePolicyLatchesUnsafeSamples(t *testing.T) {
	rules, s := faultScenario(t)
	for _, reading := range []ClockReading{
		{UTC: 106, Elapsed: 1, Safe: true},
		{UTC: 101, Elapsed: 1, Safe: false},
		{UTC: 101, Elapsed: 0, Safe: true},
	} {
		m, err := NewUncertainModel(rules, s.Initial, s.Start, 2)
		requireFaultStep(t, err)
		requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: 101, Elapsed: 1, Safe: true}))
		if err := m.faults.Publish(0, reading); err == nil {
			t.Fatal("accepts an unsafe clock sample")
		}
		if err := m.faults.Publish(0, ClockReading{UTC: 114, Elapsed: 14, Safe: true}); err == nil {
			t.Fatal("later sample clears unsafe latch")
		}
	}
}

func TestConservativePolicyRetainsReleaseAfterRollbackQuarantine(t *testing.T) {
	rules, s := faultScenario(t)
	m, err := NewUncertainModel(rules, s.Initial, s.Start, 2)
	requireFaultStep(t, err)
	requireFaultStep(t, m.faults.Publish(0, ClockReading{UTC: 114, Elapsed: 14, Safe: true}))
	requireFaultStep(t, m.faults.Crash(0))
	requireFaultStep(t, m.RestartAgent(0, true))
	if err := m.faults.Publish(0, ClockReading{UTC: 115, Elapsed: 15, Safe: true}); err == nil {
		t.Fatal("stale snapshot publishes after quarantine")
	}
	if m.faults.observer.public != 1 {
		t.Fatal("rollback recalls released share")
	}
}

func TestIncludesUnsentIntentAndPermitsReleaseAtB0(t *testing.T) {
	m, s, _ := uncertainFixture(t)
	if m.client.Bounds() != (Uncertainty{B0: 110, Bmax: 210}) {
		t.Fatal("omits durable unsent intent")
	}
	m.LoseClient()
	requireFaultStep(t, m.faults.Crash(2))
	evidence, err := recoveryEvidence(m.faults, [3]bool{true, true, true}, s.Observe)
	requireFaultStep(t, err)
	if !evidence.ContentAvailable {
		t.Fatal("uncertainty falsely prohibits release at B0")
	}
}

func TestRetainsReleasedSharesAfterRestartCancelAndRenewal(t *testing.T) {
	m, s, intent := uncertainFixture(t)
	requireFaultStep(t, m.faults.Publish(0, s.Observe))
	requireFaultStep(t, m.faults.Crash(0))
	requireFaultStep(t, m.RestartAgent(0, false))
	if err := m.Deliver(0, intent, s.Observe, true, true); err == nil {
		t.Fatal("ever-released agent renews content key")
	}
	if m.faults.observer.public != 1 {
		t.Fatal("restart recalls observer share")
	}
	m.Cancel()
	if err := m.Deliver(1, intent, s.Observe, true, true); err == nil {
		t.Fatal("canceled client renews")
	}
	requireFaultStep(t, m.faults.Publish(1, s.Observe))
	if !m.faults.ReleaseAvailable() || m.faults.observer.public != 3 {
		t.Fatal("cancellation recalls observer shares")
	}
}

func TestQuarantinesStaleRestartWithoutRecallingObserverShares(t *testing.T) {
	m, s, intent := uncertainFixture(t)
	requireFaultStep(t, m.faults.Publish(0, s.Observe))
	requireFaultStep(t, m.faults.Crash(0))
	requireFaultStep(t, m.RestartAgent(0, true))
	if err := m.faults.Publish(0, s.Observe); err == nil {
		t.Fatal("stale state publishes")
	}
	if err := m.Deliver(0, intent, s.Observe, true, true); err == nil {
		t.Fatal("stale state renews")
	}
	if m.faults.observer.public != 1 {
		t.Fatal("quarantine recalls public copy")
	}
}

func TestRejectsConflictingRetriesAndInvalidTransitions(t *testing.T) {
	m, s, intent := uncertainFixture(t)
	conflict := cloneIntent(intent)
	conflict.Bytes[0] = 'X'
	if err := m.Deliver(0, conflict, s.Renew, true, true); err == nil {
		t.Fatal("accepts changed signed bytes")
	}
	conflict = cloneIntent(intent)
	conflict.State.Deadline++
	if err := m.Deliver(0, conflict, s.Renew, true, true); err == nil {
		t.Fatal("accepts conflicting deadline")
	}
	if err := m.Deliver(0, intent, s.Renew, false, true); err == nil {
		t.Fatal("receipts precede persistence")
	}
	if err := m.Deliver(9, intent, s.Renew, true, true); err == nil {
		t.Fatal("accepts unknown agent")
	}
	requireFaultStep(t, m.faults.Crash(0))
	if err := m.Deliver(0, intent, s.Renew, true, true); err == nil {
		t.Fatal("offline agent accepts")
	}
	if err := m.RestartAgent(9, false); err == nil {
		t.Fatal("restarts unknown agent")
	}
	m.LoseClient()
	untransmitted := cloneIntent(intent)
	untransmitted.State.Sequence++
	if err := m.Deliver(1, untransmitted, s.Renew, true, true); err == nil {
		t.Fatal("lost client creates another intent")
	}
}

func TestInFlightIntentArrivesAfterPermanentClientLoss(t *testing.T) {
	m, s, intent := uncertainFixture(t)
	requireFaultStep(t, m.Transmit(intent))
	m.LoseClient()
	if err := m.Transmit(intent); err == nil {
		t.Fatal("lost client transmits again")
	}
	requireFaultStep(t, m.Deliver(0, intent, s.Renew, true, true))
	if m.faults.Accepted()[0] != intent.State {
		t.Fatal("client loss drops in-flight signed request")
	}
	requireFaultStep(t, m.faults.Crash(2))
	evidence, err := recoveryEvidence(m.faults, [3]bool{true, true, true}, ClockReading{UTC: 210, Elapsed: 110, Safe: true})
	requireFaultStep(t, err)
	if !evidence.ContentAvailable {
		t.Fatal("in-flight acceptance exceeds Bmax")
	}
}

func TestRejectsUnsafeInitialStateAndAllowanceOverflow(t *testing.T) {
	rules, s := faultScenario(t)
	if _, err := NewUncertainModel(rules, s.Initial, ClockReading{}, 0); err == nil {
		t.Fatal("accepts unsafe initial clock")
	}
	if _, err := (Allowance{UTCError: 1}).ReleaseBy(math.MaxInt64); err == nil {
		t.Fatal("overflows clock allowance")
	}
	if _, err := (Allowance{Publication: 1}).ReleaseBy(math.MaxInt64); err == nil {
		t.Fatal("overflows publication allowance")
	}
}

func TestRejectsDelayedOlderIntentAfterLaterAcceptance(t *testing.T) {
	m, s, first := uncertainFixture(t)
	second := cloneIntent(first)
	second.State.Sequence++
	second.State.Deadline = 300
	second.Bytes = []byte("later signed intent")
	requireFaultStep(t, m.client.Issue(second, true))
	requireFaultStep(t, m.Deliver(0, second, s.Renew, true, true))
	if err := m.Deliver(0, first, ClockReading{UTC: 102, Elapsed: 2, Safe: true}, true, true); err == nil {
		t.Fatal("delayed intent overwrites later sequence")
	}
	if m.faults.Accepted()[0] != second.State {
		t.Fatal("replay changes durable authority")
	}
}

func TestRenewalAndReleaseRaceKeepsIrreversibleBoundary(t *testing.T) {
	for _, publishFirst := range []bool{false, true} {
		m, s, intent := uncertainFixture(t)
		if publishFirst {
			requireFaultStep(t, m.faults.Publish(0, s.Observe))
		}
		if err := m.Deliver(0, intent, s.Observe, true, true); err == nil {
			t.Fatal("renewal wins at release boundary")
		}
		requireFaultStep(t, m.faults.Publish(0, s.Observe))
		requireFaultStep(t, m.faults.Crash(0))
		requireFaultStep(t, m.RestartAgent(0, false))
		if m.faults.observer.public != 1 || !m.faults.agents[0].durable.released {
			t.Fatal("race loses durable release history")
		}
	}
}
