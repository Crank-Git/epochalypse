package agent

import (
	"errors"
	"time"
)

var ErrClock = errors.New("agent clock assessment is unavailable")

const (
	CandidateEpsilonSeconds   int64 = 2
	CandidateSampleGapSeconds int64 = 5
	CandidateWorkSeconds      int64 = 25
	CandidateAllowanceSeconds int64 = 2*CandidateEpsilonSeconds + CandidateSampleGapSeconds + CandidateWorkSeconds
)

// Assessment contains operator-pinned clock evidence for one capture epoch.
// A fresh assessment follows suspend, a step, or source loss.
type Assessment struct {
	Epoch                string
	Implementation       string
	Provenance           string
	SourceIdentities     []string
	ReferenceEvidence    string
	IndependenceEvidence string
	LeapPolicy           string
	AssessedElapsed      time.Duration
	BaseUncertainty      time.Duration
	RatePPM              uint32
}

// Capture contains one UTC sample and elapsed readings from the assessed epoch.
type Capture struct {
	Epoch           string
	UTCSeconds      int64
	CapturedElapsed time.Duration
	DecisionElapsed time.Duration
}

// Reading retains the conservative UTC bound for this decision only.
type Reading struct{ lower int64 }

// LowerUTC returns the conservative UTC boundary for this decision.
func (r Reading) LowerUTC() int64 { return r.lower }

// AssessClock checks the candidate profile after authoritative state validation.
// Operator premises establish reference correctness and the configured error bound.
func AssessClock(a Assessment, c Capture) (Reading, error) {
	if !assessmentComplete(a) || !captureCurrent(a, c) || c.UTCSeconds < CandidateEpsilonSeconds {
		return Reading{}, ErrClock
	}
	return Reading{lower: c.UTCSeconds - CandidateEpsilonSeconds}, nil
}

func assessmentComplete(a Assessment) bool {
	if a.Epoch == "" || a.Implementation == "" || a.Provenance == "" || a.ReferenceEvidence == "" ||
		a.IndependenceEvidence == "" || a.LeapPolicy == "" {
		return false
	}
	if a.BaseUncertainty < 0 || a.BaseUncertainty > 500*time.Millisecond || a.RatePPM > 100 {
		return false
	}
	return distinctSources(a.SourceIdentities)
}

func distinctSources(sources []string) bool {
	if len(sources) < 2 || len(sources) > 8 {
		return false
	}
	seen := make(map[string]bool, len(sources))
	for _, s := range sources {
		if s == "" || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}

func captureCurrent(a Assessment, c Capture) bool {
	if c.Epoch != a.Epoch || a.AssessedElapsed < 0 || c.CapturedElapsed < a.AssessedElapsed || c.DecisionElapsed < c.CapturedElapsed {
		return false
	}
	return c.CapturedElapsed-a.AssessedElapsed <= 60*time.Second && c.DecisionElapsed-c.CapturedElapsed <= 250*time.Millisecond
}
