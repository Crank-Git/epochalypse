package agent

import (
	"testing"
	"time"
)

func clockFixture() (Assessment, Capture) {
	return Assessment{
		Epoch: "epoch1", Implementation: "synthetic-test-only", Provenance: "test fixture",
		SourceIdentities: []string{"reference1", "reference2"}, ReferenceEvidence: "test reference assessment",
		IndependenceEvidence: "test independence assessment", LeapPolicy: "UTC, no unresolved leap",
		AssessedElapsed: time.Second, BaseUncertainty: 500 * time.Millisecond, RatePPM: 100,
	}, Capture{Epoch: "epoch1", UTCSeconds: 100, CapturedElapsed: 61 * time.Second, DecisionElapsed: 61*time.Second + 250*time.Millisecond}
}

func TestClockAcceptsCandidateCeilings(t *testing.T) {
	a, c := clockFixture()
	r, err := AssessClock(a, c)
	if err != nil || r.LowerUTC() != 98 {
		t.Fatalf("candidate profile: lower=%d error=%v", r.LowerUTC(), err)
	}
	if CandidateAllowanceSeconds != 34 {
		t.Fatal("candidate allowance differs")
	}
	a.BaseUncertainty = 0
	a.RatePPM = 0
	c.UTCSeconds = 2
	if r, err = AssessClock(a, c); err != nil || r.LowerUTC() != 0 {
		t.Fatal("rejects conservative lower boundary")
	}
}

func TestClockRejectsMissingOrExpiredEvidence(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Assessment, *Capture)
	}{
		{"epoch", func(a *Assessment, c *Capture) { c.Epoch = "old" }},
		{"missing epoch", func(a *Assessment, c *Capture) { a.Epoch = ""; c.Epoch = "" }},
		{"implementation", func(a *Assessment, c *Capture) { a.Implementation = "" }},
		{"provenance", func(a *Assessment, c *Capture) { a.Provenance = "" }},
		{"references", func(a *Assessment, c *Capture) { a.SourceIdentities = nil }},
		{"excess references", func(a *Assessment, c *Capture) { a.SourceIdentities = make([]string, 9) }},
		{"duplicate references", func(a *Assessment, c *Capture) { a.SourceIdentities = []string{"a", "a"} }},
		{"empty reference", func(a *Assessment, c *Capture) { a.SourceIdentities = []string{"a", ""} }},
		{"reference evidence", func(a *Assessment, c *Capture) { a.ReferenceEvidence = "" }},
		{"independence", func(a *Assessment, c *Capture) { a.IndependenceEvidence = "" }},
		{"leap", func(a *Assessment, c *Capture) { a.LeapPolicy = "" }},
		{"age", func(a *Assessment, c *Capture) {
			c.CapturedElapsed += time.Nanosecond
			c.DecisionElapsed += time.Nanosecond
		}},
		{"capture delay", func(a *Assessment, c *Capture) { c.DecisionElapsed += time.Nanosecond }},
		{"reset epoch", func(a *Assessment, c *Capture) { c.CapturedElapsed = 0 }},
		{"reverse capture", func(a *Assessment, c *Capture) { c.DecisionElapsed = c.CapturedElapsed - time.Nanosecond }},
		{"negative assessment", func(a *Assessment, c *Capture) { a.AssessedElapsed = -1 }},
		{"base uncertainty", func(a *Assessment, c *Capture) { a.BaseUncertainty += time.Nanosecond }},
		{"negative uncertainty", func(a *Assessment, c *Capture) { a.BaseUncertainty = -1 }},
		{"rate", func(a *Assessment, c *Capture) { a.RatePPM++ }},
		{"UTC underflow", func(a *Assessment, c *Capture) { c.UTCSeconds = 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, c := clockFixture()
			tt.change(&a, &c)
			if _, err := AssessClock(a, c); err == nil {
				t.Fatal("accepts invalid clock evidence")
			}
		})
	}
}
