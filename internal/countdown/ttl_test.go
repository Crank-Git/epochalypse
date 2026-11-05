package countdown

import (
	"testing"
	"time"
)

func TestTimingPreservesPrecisionAndCapsClockRollback(t *testing.T) {
	event := testNow.Truncate(time.Second).Add(time.Duration(MaxTTL) * time.Second)
	r := Record{EventAt: event}
	for _, tc := range []struct {
		at  time.Time
		ttl uint32
	}{
		{event, 0}, {event.Add(time.Hour), 0},
		{event.Add(-time.Nanosecond), 0},
		{event.Add(-time.Second), 1},
		{event.Add(-1500 * time.Millisecond), 1},
		{event.Add(-time.Duration(MaxTTL) * time.Second), MaxTTL},
		{event.Add(-time.Duration(MaxTTL)*time.Second - time.Nanosecond), MaxTTL},
		{event.Add(-time.Duration(MaxTTL)*time.Second + time.Nanosecond), MaxTTL - 1},
	} {
		timing := r.Timing(tc.at)
		if timing.AuthoritativeTTL != tc.ttl || !timing.ServerNow.Equal(tc.at) || timing.ServerNow.Location() != time.UTC {
			t.Fatalf("timing=%+v want TTL %d at %s", timing, tc.ttl, tc.at)
		}
	}
}
