package compatibility

import (
	"fmt"
	"math"
)

func roundAtOrAfter(requested, genesis, period int64) (uint64, int64, error) {
	if requested < 0 || genesis < 0 || period <= 0 {
		return 0, 0, fmt.Errorf("invalid time or period")
	}
	if requested <= genesis {
		return 1, genesis, nil
	}
	delta := requested - genesis
	offset := delta / period
	if delta%period != 0 {
		offset++
	}
	if offset > (math.MaxInt64-genesis)/period {
		return 0, 0, fmt.Errorf("effective time exceeds UTC seconds")
	}
	return uint64(offset) + 1, genesis + offset*period, nil // #nosec G115 -- The input checks keep the offset nonnegative.
}
