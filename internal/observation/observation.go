// Package observation records DNS responses without a confidentiality claim.
package observation

import (
	"time"
)

// Clock records the configured offset separately from a measured interval.
type Clock struct {
	OffsetNanoseconds *int64 `json:"offset_nanoseconds"`
	Source            string `json:"source"`
	LowerNanoseconds  *int64 `json:"lower_nanoseconds,omitempty"`
	UpperNanoseconds  *int64 `json:"upper_nanoseconds,omitempty"`
}

// Target identifies a resolver from trusted installation configuration.
type Target struct {
	ID            string `json:"id"`
	Address       string `json:"address,omitempty"`
	Version       string `json:"version"`
	VersionSource string `json:"version_source"`
	Clock         Clock  `json:"clock"`
}

// Flags preserves the available DNS response flags.
type Flags struct {
	AD bool `json:"ad"`
	CD bool `json:"cd"`
	AA bool `json:"aa"`
	RA bool `json:"ra"`
	RD bool `json:"rd"`
	TC bool `json:"tc"`
	DO bool `json:"do"`
}

// Result preserves one resolver response and its environment.
type Result struct {
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	Resolver  Target    `json:"resolver"`
	Started   time.Time `json:"started"`
	Completed time.Time `json:"completed"`
	Transport string    `json:"transport"`
	Rcode     *uint16   `json:"rcode"`
	Flags     *Flags    `json:"flags"`
	Error     string    `json:"error,omitempty"`
}
