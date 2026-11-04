// Package observation records DNS responses without a confidentiality claim.
package observation

import (
	"context"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/authority"
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
	Address       string `json:"address"`
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
	Resolver  Target    `json:"resolver"`
	Started   time.Time `json:"started"`
	Completed time.Time `json:"completed"`
	Transport string    `json:"transport"`
	Rcode     *uint16   `json:"rcode"`
	Flags     *Flags    `json:"flags"`
	Error     string    `json:"error,omitempty"`
}

// Observe queries one configured resolver for a public TXT record.
func Observe(ctx context.Context, name string, target Target) Result {
	result := Result{Version: 1, Resolver: target, Started: time.Now().UTC()}
	request := dns.NewMsg(name, dns.TypeTXT)
	request.RecursionDesired = true
	request.Security = true
	request.UDPSize = 1232
	response, transport, err := authority.Query(ctx, target.Address, request)
	result.Completed = time.Now().UTC()
	result.Transport = transport
	if err != nil {
		result.Error = "resolver_unavailable"
		return result
	}
	code := response.Rcode
	result.Rcode = &code
	result.Flags = &Flags{AD: response.AuthenticatedData, CD: response.CheckingDisabled, AA: response.Authoritative, RA: response.RecursionAvailable, RD: response.RecursionDesired, TC: response.Truncated, DO: response.Security}
	return result
}
