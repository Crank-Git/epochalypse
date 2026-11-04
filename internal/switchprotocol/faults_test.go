package switchprotocol

import "testing"

func requireFaultStep(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func faultScenario(t *testing.T) (Rules, FaultScenario) {
	t.Helper()
	rules, old := fixture(t)
	next := old
	next.Sequence++
	next.Deadline = 200
	return rules, FaultScenario{Initial: old, Next: next,
		Start:   ClockReading{UTC: 100, Safe: true},
		Renew:   ClockReading{UTC: 101, Elapsed: 1, Safe: true},
		Observe: ClockReading{UTC: 110, Elapsed: 10, Safe: true}}
}

func TestEnumeratesPartialRenewalAndLostReceipts(t *testing.T) {
	rules, s := faultScenario(t)
	results, err := ExploreRenewalFaults(rules, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 64 {
		t.Fatalf("got %d schedules, want 64", len(results))
	}
	confirmed, delayed := 0, 0
	for i, result := range results {
		if int(result.Schedule) != i {
			t.Fatal("changes deterministic schedule order")
		}
		if result.Confirmed == s.Next {
			confirmed++
		}
		if result.Confirmed == s.Initial && !result.ReleaseAvailable {
			delayed++
		}
		for agent, accepted := range result.Accepted {
			want := s.Initial
			if result.Persisted&(1<<agent) != 0 {
				want = s.Next
			}
			if accepted != want {
				t.Fatal("restart loses the presumed durable state")
			}
		}
	}
	if confirmed != 1 || delayed != 31 {
		t.Fatalf("confirmed=%d delayed=%d, want 1 and 31", confirmed, delayed)
	}
}

func TestRetainsPublishedShareAfterRenewalAndRestart(t *testing.T) {
	rules, s := faultScenario(t)
	m, err := NewFaultModel(rules, s.Initial, s.Start, 0)
	if err != nil {
		t.Fatal(err)
	}
	// A shorter accepted deadline makes agent zero eligible before the other agents.
	short := s.Next
	short.Deadline = 101
	short.Grace = 0
	if err := m.CheckIn(0, short, s.Renew, true, true); err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(0, s.Renew); err != nil {
		t.Fatal(err)
	}
	if m.ReleaseAvailable() {
		t.Fatal("one share meets the threshold")
	}
	for _, agent := range []int{1, 2} {
		if err := m.CheckIn(agent, s.Next, s.Renew, true, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Crash(0); err != nil {
		t.Fatal(err)
	}
	if err := m.Restart(0, true); err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(1, ClockReading{UTC: 210, Elapsed: 110, Safe: true}); err != nil {
		t.Fatal(err)
	}
	if !m.ReleaseAvailable() {
		t.Fatal("loses a public share after renewal or rollback")
	}
	if m.Confirmed() != s.Initial {
		t.Fatal("confirms mismatched accepted states")
	}
}

func TestDetectsEarlyReleaseAfterStaleSnapshotRollback(t *testing.T) {
	rules, s := faultScenario(t)
	m, err := NewFaultModel(rules, s.Initial, s.Start, 0)
	requireFaultStep(t, err)
	for agent := 0; agent < 3; agent++ {
		requireFaultStep(t, m.CheckIn(agent, s.Next, s.Renew, true, true))
	}
	if m.Confirmed() != s.Next {
		t.Fatal("loses three durable receipts")
	}
	for _, agent := range []int{0, 1} {
		requireFaultStep(t, m.Crash(agent))
		requireFaultStep(t, m.Restart(agent, true))
		requireFaultStep(t, m.Publish(agent, s.Observe))
	}
	if !m.ReleaseAvailable() || m.Confirmed().Deadline != 200 {
		t.Fatal("fails to expose early release under stale rollback")
	}
}

func TestSnapshotPreservesReleaseAndUnsafeLatches(t *testing.T) {
	for _, safe := range []bool{true, false} {
		rules, s := faultScenario(t)
		m, err := NewFaultModel(rules, s.Initial, s.Start, 2)
		requireFaultStep(t, err)
		clock := s.Observe
		clock.Safe = safe
		err = m.Publish(0, clock)
		if (err == nil) != safe {
			t.Fatal("changes unsafe clock result")
		}
		requireFaultStep(t, m.Crash(0))
		requireFaultStep(t, m.Restart(0, false))
		// A backward sample stays within tolerance but cannot clear either latch.
		clock.UTC = 109
		clock.Safe = true
		if err := m.CheckIn(0, s.Next, clock, true, true); err == nil {
			t.Fatal("restart clears a durable latch")
		}
		if m.Accepted()[0] != s.Initial {
			t.Fatal("rejected check-in changes accepted state")
		}
	}
}

func TestRejectsUnavailableAgentsAndInvalidFaultSteps(t *testing.T) {
	rules, s := faultScenario(t)
	m, err := NewFaultModel(rules, s.Initial, s.Start, 0)
	requireFaultStep(t, err)
	for _, agent := range []int{-1, 3} {
		if m.Crash(agent) == nil || m.Restart(agent, false) == nil {
			t.Fatal("accepts unknown agent")
		}
	}
	if m.Restart(0, false) == nil {
		t.Fatal("restarts an online agent")
	}
	if m.CheckIn(0, s.Next, s.Renew, false, true) == nil {
		t.Fatal("attests to volatile state")
	}
	requireFaultStep(t, m.Crash(0))
	if m.Crash(0) == nil || m.Publish(0, s.Observe) == nil || m.CheckIn(0, s.Next, s.Renew, true, true) == nil {
		t.Fatal("uses an offline agent")
	}
	if _, err := NewFaultModel(Rules{}, s.Initial, s.Start, 0); err == nil {
		t.Fatal("accepts invalid enrollment")
	}
}

func TestRejectsInvalidFaultScenarios(t *testing.T) {
	rules, s := faultScenario(t)
	for _, change := range []func(*FaultScenario){
		func(s *FaultScenario) { s.Start.Safe = false },
		func(s *FaultScenario) { s.Next.Sequence = s.Initial.Sequence },
		func(s *FaultScenario) { s.Observe.Safe = false },
	} {
		invalid := s
		change(&invalid)
		if _, err := ExploreRenewalFaults(rules, invalid); err == nil {
			t.Fatal("accepts invalid fault scenario")
		}
	}
}

func TestPreservesCompetingRenewalEvidence(t *testing.T) {
	rules, s := faultScenario(t)
	m, err := NewFaultModel(rules, s.Initial, s.Start, 0)
	requireFaultStep(t, err)
	other := s.Next
	other.Deadline = 300
	requireFaultStep(t, m.CheckIn(0, s.Next, s.Renew, true, true))
	for _, agent := range []int{1, 2} {
		requireFaultStep(t, m.CheckIn(agent, other, s.Renew, true, true))
	}
	if m.CheckIn(0, other, s.Renew, true, true) == nil {
		t.Fatal("accepts concurrent sequence conflict")
	}
	if m.Confirmed() != s.Initial {
		t.Fatal("combines receipts for different deadlines")
	}
}
