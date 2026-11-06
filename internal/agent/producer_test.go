package agent

import (
	"testing"
	"time"
)

func TestLabClockUsesDecisionLocalLowerBound(t *testing.T) {
	now := time.Unix(10_000, 0)
	read := NewLabClock(func() time.Time { return now })
	reading, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if reading.LowerUTC() != now.Unix()-CandidateEpsilonSeconds {
		t.Fatalf("lower UTC = %d, want %d", reading.LowerUTC(), now.Unix()-CandidateEpsilonSeconds)
	}
	now = now.Add(time.Second)
	updated, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if updated.LowerUTC() != reading.LowerUTC()+1 {
		t.Fatal("lab clock did not take a fresh sample")
	}
}

func TestLabClockRejectsTimeBeforeCandidateEpsilon(t *testing.T) {
	read := NewLabClock(func() time.Time { return time.Unix(CandidateEpsilonSeconds-1, 0) })
	if _, err := read(); err != ErrClock {
		t.Fatalf("clock error = %v, want ErrClock", err)
	}
}
