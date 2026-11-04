package switchprotocol

import (
	"math"
	"testing"
)

func modelAgent(t *testing.T, utc int64, tolerance uint64) (*ModelAgent, State) {
	t.Helper()
	_, state := fixture(t)
	a, err := NewModelAgent(state, ClockReading{UTC: utc, Safe: true}, tolerance)
	if err != nil {
		t.Fatal(err)
	}
	return a, state
}

func TestAcceptsIncreasingSequenceBeforeBoundary(t *testing.T) {
	a, next := modelAgent(t, 100, 0)
	next.Sequence += 2
	next.Deadline = 200
	if err := a.CheckIn(next, ClockReading{UTC: 109, Elapsed: 9, Safe: true}); err != nil {
		t.Fatal(err)
	}
	if a.Accepted() != next {
		t.Fatal("loses accepted state")
	}
}

func TestRejectsReplayStaleAndConflict(t *testing.T) {
	for _, change := range []func(*State){
		func(s *State) {},
		func(s *State) { s.Sequence-- },
		func(s *State) { s.Deadline++ },
		func(s *State) { s.Sequence++; s.SwitchID = "other" },
		func(s *State) { s.Sequence++; s.ProtocolVersion++ },
		func(s *State) { s.Sequence++; s.ContentDigest[0]++ },
		func(s *State) { s.Sequence++; s.Deadline = math.MaxInt64 },
	} {
		a, old := modelAgent(t, 100, 0)
		next := old
		change(&next)
		if err := a.CheckIn(next, ClockReading{UTC: 101, Elapsed: 1, Safe: true}); err == nil {
			t.Fatal("accepts rejected state")
		}
		if a.Accepted() != old {
			t.Fatal("changes accepted state on rejection")
		}
	}
	_, old := modelAgent(t, 100, 0)
	old.Sequence = math.MaxUint64
	a, _ := NewModelAgent(old, ClockReading{UTC: 100, Safe: true}, 0)
	next := old
	if err := a.CheckIn(next, ClockReading{UTC: 101, Elapsed: 1, Safe: true}); err == nil {
		t.Fatal("wraps sequence")
	}
}

func TestRejectsRenewalAtReleaseBoundary(t *testing.T) {
	for _, sample := range []struct {
		utc     int64
		elapsed uint64
	}{{109, 9}, {110, 10}, {111, 11}} {
		utc := sample.utc
		a, old := modelAgent(t, 100, 0)
		next := old
		next.Sequence++
		next.Deadline = 200
		err := a.CheckIn(next, ClockReading{UTC: utc, Elapsed: sample.elapsed, Safe: true})
		if (err == nil) != (utc < 110) {
			t.Fatalf("boundary %d: %v", utc, err)
		}
		if err != nil && a.Accepted() != old {
			t.Fatal("changes rejected state")
		}
	}
}

func TestRetainsReleaseBoundaryAfterBackwardStep(t *testing.T) {
	a, next := modelAgent(t, 109, 2)
	eligible, err := a.ReleaseEligible(ClockReading{UTC: 110, Elapsed: 1, Safe: true})
	if err != nil || !eligible {
		t.Fatalf("misses release: %v", err)
	}
	next.Sequence++
	next.Deadline = 200
	if err := a.CheckIn(next, ClockReading{UTC: 109, Elapsed: 2, Safe: true}); err == nil {
		t.Fatal("reopens release boundary")
	}
	eligible, err = a.ReleaseEligible(ClockReading{UTC: 109, Elapsed: 2, Safe: true})
	if err != nil || !eligible {
		t.Fatalf("loses release: %v", err)
	}
}

func TestModelsBoundedClockSteps(t *testing.T) {
	for _, utc := range []int64{99, 101, 103} {
		a, next := modelAgent(t, 100, 2)
		next.Sequence++
		next.Deadline = 200
		if err := a.CheckIn(next, ClockReading{UTC: utc, Elapsed: 1, Safe: true}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRejectsUnsafeClockAndRetainsState(t *testing.T) {
	for _, reading := range []ClockReading{
		{UTC: 104, Elapsed: 1, Safe: true},
		{UTC: 98, Elapsed: 1, Safe: true},
		{UTC: 101, Elapsed: 1},
		{UTC: math.MaxInt64, Elapsed: math.MaxUint64, Safe: true},
	} {
		a, old := modelAgent(t, 100, 2)
		next := old
		next.Sequence++
		next.Deadline = 200
		if err := a.CheckIn(next, reading); err == nil {
			t.Fatal("accepts unsafe clock")
		}
		if a.Accepted() != old {
			t.Fatal("changes accepted state")
		}
		if _, err := a.ReleaseEligible(ClockReading{UTC: 102, Elapsed: 2, Safe: true}); err == nil {
			t.Fatal("clears unsafe clock without review")
		}
	}
	a, _ := modelAgent(t, 100, 2)
	if _, err := a.ReleaseEligible(ClockReading{UTC: 101, Elapsed: 1, Safe: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReleaseEligible(ClockReading{UTC: 101, Elapsed: 0, Safe: true}); err == nil {
		t.Fatal("accepts backward elapsed time")
	}
}

func TestRejectsBoundaryOverflow(t *testing.T) {
	_, state := fixture(t)
	for _, tc := range []struct {
		deadline int64
		grace    uint64
		valid    bool
	}{
		{math.MaxInt64, 1, false}, {0, math.MaxUint64, false},
		{math.MinInt64, math.MaxUint64, true}, {-1, 1, true}, {math.MaxInt64, 0, true},
	} {
		state.Deadline = tc.deadline
		state.Grace = tc.grace
		_, err := NewModelAgent(state, ClockReading{UTC: math.MinInt64, Safe: true}, 0)
		if (err == nil) != tc.valid {
			t.Fatalf("boundary %+v: %v", tc, err)
		}
	}
	if _, err := NewModelAgent(state, ClockReading{}, 0); err == nil {
		t.Fatal("accepts unsafe initial clock")
	}
}

func TestPreservesFirstAcceptedConcurrentSequence(t *testing.T) {
	a, next := modelAgent(t, 100, 0)
	next.Sequence++
	next.Deadline = 200
	reading := ClockReading{UTC: 101, Elapsed: 1, Safe: true}
	if err := a.CheckIn(next, reading); err != nil {
		t.Fatal(err)
	}
	conflict := next
	conflict.Deadline = 300
	if err := a.CheckIn(conflict, reading); err == nil {
		t.Fatal("replaces accepted sequence")
	}
	if a.Accepted() != next {
		t.Fatal("loses first accepted state")
	}
}

func TestReportsLocalReleaseEligibility(t *testing.T) {
	for _, sample := range []struct {
		utc     int64
		elapsed uint64
	}{{109, 9}, {110, 10}, {111, 11}} {
		utc := sample.utc
		a, _ := modelAgent(t, 100, 0)
		eligible, err := a.ReleaseEligible(ClockReading{UTC: utc, Elapsed: sample.elapsed, Safe: true})
		if err != nil || eligible != (utc >= 110) {
			t.Fatalf("eligibility at %d: %v, %v", utc, eligible, err)
		}
	}
}
