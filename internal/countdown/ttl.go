package countdown

import "time"

// Timing contains one precise server clock sample and its authoritative TTL.
type Timing struct {
	ServerNow        time.Time `json:"server_now"`
	AuthoritativeTTL uint32    `json:"authoritative_ttl"`
}

// Timing returns advisory whole seconds without changing the retained event.
func (r Record) Timing(now time.Time) Timing {
	remaining := r.EventAt.Sub(now) / time.Second
	return Timing{ServerNow: now.UTC(), AuthoritativeTTL: uint32(max(0, min(remaining, MaxTTL)))} // #nosec G115 -- The clamp bounds the conversion to the supported TTL range.
}
