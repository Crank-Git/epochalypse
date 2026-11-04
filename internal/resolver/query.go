// Package resolver queries trusted configured DNS resolvers.
package resolver

import (
	"codeberg.org/miekg/dns"
	"context"
	"github.com/Crank-Git/epochalypse/internal/authority"
	"github.com/Crank-Git/epochalypse/internal/observation"
	"time"
)

// Observe queries one configured resolver for a public TXT record.
func Observe(ctx context.Context, name string, target observation.Target) observation.Result {
	result := observation.Result{Version: 1, Name: name, Resolver: target, Started: time.Now().UTC()}
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
	result.Flags = &observation.Flags{AD: response.AuthenticatedData, CD: response.CheckingDisabled, AA: response.Authoritative, RA: response.RecursionAvailable, RD: response.RecursionDesired, TC: response.Truncated, DO: response.Security}
	return result
}
