package agent

import (
	"errors"
	"math"
	"time"
)

// NewLabClock returns a synthetic clock for explicit local experiments.
func NewLabClock(now func() time.Time) func() (Reading, error) {
	return func() (Reading, error) {
		if now == nil {
			return Reading{}, ErrClock
		}
		seconds := now().Unix()
		if seconds < CandidateEpsilonSeconds {
			return Reading{}, ErrClock
		}
		return Reading{lower: seconds - CandidateEpsilonSeconds}, nil
	}
}

func validClockNumber(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

var errChronyEvidence = errors.New("chrony evidence is not ready")
