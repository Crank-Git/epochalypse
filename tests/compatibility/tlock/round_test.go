package compatibility

import (
	"math"
	"testing"
)

func TestSelectsFirstRoundAtOrAfterTime(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		requested, genesis, period int64
		round                      uint64
		effective                  int64
		invalid                    bool
	}{
		{"before genesis", 99, 100, 3, 1, 100, false},
		{"at genesis", 100, 100, 3, 1, 100, false},
		{"exact round", 103, 100, 3, 2, 103, false},
		{"between rounds", 104, 100, 3, 3, 106, false},
		{"last representable round", math.MaxInt64, 0, 1, uint64(math.MaxInt64) + 1, math.MaxInt64, false},
		{"negative request", -1, 100, 3, 0, 0, true},
		{"negative genesis", 100, -1, 3, 0, 0, true},
		{"zero period", 100, 100, 0, 0, 0, true},
		{"negative period", 100, 100, -1, 0, 0, true},
		{"effective time overflow", math.MaxInt64, 0, 3, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			round, effective, err := roundAtOrAfter(tc.requested, tc.genesis, tc.period)
			if (err != nil) != tc.invalid {
				t.Fatalf("error = %v, invalid = %v", err, tc.invalid)
			}
			if tc.invalid {
				return
			}
			if round != tc.round || effective != tc.effective {
				t.Fatalf("got (%d, %d), want (%d, %d)", round, effective, tc.round, tc.effective)
			}
			if effective < tc.requested || (round > 1 && effective-tc.period >= tc.requested) {
				t.Fatal("round does not satisfy the first-round boundary")
			}
		})
	}
}
