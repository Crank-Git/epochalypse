package switchprotocol

import "testing"

func recoveryScenario(t *testing.T) (Rules, RecoveryScenario) {
	t.Helper()
	rules, s := faultScenario(t)
	return rules, RecoveryScenario{FaultScenario: s, Ciphertext: [3]bool{true, true, true},
		Later: ClockReading{UTC: 210, Elapsed: 110, Safe: true}}
}

func TestRecoversBaselineAfterMainLossAndEachAgentOutage(t *testing.T) {
	rules, s := recoveryScenario(t)
	results, err := ExploreRecoveryFaults(rules, s)
	requireFaultStep(t, err)
	if len(results) != 192 {
		t.Fatalf("got %d schedules, want 192", len(results))
	}
	for outage := 0; outage < 3; outage++ {
		r := results[outage]
		if r.Schedule != 0 || r.Outage != outage || r.Confirmed != s.Initial {
			t.Fatal("changes the baseline schedule")
		}
		if r.Observed.PublicShares != 7&^(1<<outage) || !r.Observed.ContentAvailable {
			t.Fatalf("baseline outage %d lacks recovery evidence: %+v", outage, r)
		}
	}
}

func TestDetectsRecoveryDelayAcrossEveryRenewalState(t *testing.T) {
	rules, s := recoveryScenario(t)
	results, err := ExploreRecoveryFaults(rules, s)
	requireFaultStep(t, err)
	if len(results) != 192 {
		t.Fatalf("got %d schedules, want 192", len(results))
	}
	delayed := 0
	for i, r := range results {
		if int(r.Schedule) != i/3 || r.Outage != i%3 {
			t.Fatal("changes deterministic schedule order")
		}
		expected := requireRecoveryStates(t, s, r)
		wantAvailable := expected != 0 && expected&(expected-1) != 0
		if r.Observed.ContentAvailable != wantAvailable || !r.Observed.CiphertextAvailable {
			t.Fatal("changes the threshold or ciphertext requirement")
		}
		if r.Confirmed == s.Initial && !r.Observed.ContentAvailable {
			delayed++
		}
		if !r.Later.ContentAvailable || r.Later.PublicShares != 7&^(1<<r.Outage) {
			t.Fatal("fails eventual recovery at the later local boundaries")
		}
	}
	if delayed != 141 {
		t.Fatalf("got %d delayed schedules, want 141", delayed)
	}
	trace := results[10*3+2]
	if trace.Observed.PublicShares != 0 || trace.Confirmed.Deadline != 100 || !trace.Later.ContentAvailable {
		t.Fatal("hides schedule 10 with agent two unavailable")
	}
}

func requireRecoveryStates(t *testing.T, s RecoveryScenario, r RecoveryResult) uint8 {
	t.Helper()
	var expected uint8
	allReceipts := true
	for agent, state := range r.Accepted {
		stage := (r.Schedule >> (2 * agent)) & 3
		want := s.Initial
		if stage >= 2 {
			want = s.Next
		}
		if state != want {
			t.Fatal("loses durable local deadline")
		}
		if stage != 3 {
			allReceipts = false
		}
		if stage < 2 && agent != r.Outage {
			expected |= 1 << agent
		}
	}
	wantConfirmed := s.Initial
	if allReceipts {
		wantConfirmed = s.Next
	}
	if r.Confirmed != wantConfirmed || r.Observed.PublicShares != expected {
		t.Fatal("confuses owner evidence with local deadlines")
	}
	return expected
}

func TestRequiresSurvivingCiphertextForRecovery(t *testing.T) {
	rules, s := recoveryScenario(t)
	for mask := 0; mask < 8; mask++ {
		for agent := range s.Ciphertext {
			s.Ciphertext[agent] = mask&(1<<agent) != 0
		}
		results, err := ExploreRecoveryFaults(rules, s)
		requireFaultStep(t, err)
		if len(results) != 192 {
			t.Fatal("omits recovery schedules")
		}
		for _, r := range results {
			available := mask&^(1<<r.Outage) != 0
			if r.Later.CiphertextAvailable != available || r.Later.ContentAvailable != available {
				t.Fatal("recovers without a surviving ciphertext copy")
			}
		}
	}
}

func TestRetainsPublicSharesFromTheUnavailableAgent(t *testing.T) {
	rules, s := recoveryScenario(t)
	s.Retained = []PublicShare{{Agent: rules.agents[0], SwitchID: s.Initial.SwitchID,
		ProtocolVersion: s.Initial.ProtocolVersion, ContentDigest: s.Initial.ContentDigest}}
	results, err := ExploreRecoveryFaults(rules, s)
	requireFaultStep(t, err)
	if len(results) != 192 {
		t.Fatal("omits recovery schedules")
	}
	// Schedule two extends only agent zero's deadline and loses its receipt.
	r := results[2*3]
	if r.Observed.PublicShares != 7 || !r.Observed.ContentAvailable || r.Later.PublicShares != 7 {
		t.Fatal("recalls an earlier public share after renewal or outage")
	}
}

func TestRejectsInvalidRecoveryScenarios(t *testing.T) {
	rules, s := recoveryScenario(t)
	for _, change := range []func(*RecoveryScenario){
		func(s *RecoveryScenario) { s.Start.Safe = false },
		func(s *RecoveryScenario) { s.Next.Sequence = s.Initial.Sequence },
		func(s *RecoveryScenario) { s.Observe.Safe = false },
		func(s *RecoveryScenario) { s.Later.Safe = false },
		func(s *RecoveryScenario) { s.Retained = []PublicShare{{Agent: "unknown"}} },
	} {
		invalid := s
		change(&invalid)
		if _, err := ExploreRecoveryFaults(rules, invalid); err == nil {
			t.Fatal("accepts invalid recovery evidence")
		}
	}
}

func TestChecksConfirmedAndLocalRecoveryBoundaries(t *testing.T) {
	rules, original := recoveryScenario(t)
	for _, clock := range []ClockReading{
		{UTC: 109, Elapsed: 9, Safe: true},
		{UTC: 110, Elapsed: 10, Safe: true},
		{UTC: 111, Elapsed: 11, Safe: true},
	} {
		s := original
		s.Observe = clock
		results, err := ExploreRecoveryFaults(rules, s)
		requireFaultStep(t, err)
		for _, r := range results[:3] {
			if r.Observed.ContentAvailable != (clock.UTC >= 110) {
				t.Fatal("changes the confirmed recovery boundary")
			}
		}
	}
	for _, clock := range []ClockReading{
		{UTC: 209, Elapsed: 109, Safe: true},
		{UTC: 210, Elapsed: 110, Safe: true},
		{UTC: 211, Elapsed: 111, Safe: true},
	} {
		s := original
		s.Later = clock
		results, err := ExploreRecoveryFaults(rules, s)
		requireFaultStep(t, err)
		for _, r := range results[63*3:] {
			if r.Confirmed != s.Next || r.Later.ContentAvailable != (clock.UTC >= 210) {
				t.Fatal("changes the renewed local recovery boundary")
			}
		}
	}
}

func TestPreservesRecoveryDelayForALaterProposedDeadline(t *testing.T) {
	rules, s := recoveryScenario(t)
	s.Next.Deadline = 1000000
	s.Later = ClockReading{UTC: 1000010, Elapsed: 999910, Safe: true}
	results, err := ExploreRecoveryFaults(rules, s)
	requireFaultStep(t, err)
	r := results[10*3+2]
	if r.Confirmed != s.Initial || r.Observed.ContentAvailable || !r.Later.ContentAvailable {
		t.Fatal("treats eventual release as a bound at the confirmed deadline")
	}
}
