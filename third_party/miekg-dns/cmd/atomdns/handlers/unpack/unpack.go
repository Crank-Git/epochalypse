package unpack

import (
	"context"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnsctx"
)

type Unpack int

func (u *Unpack) HandlerFunc(next dns.HandlerFunc) dns.HandlerFunc {
	return dns.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
		if err := r.Unpack(); err != nil {
			log().Debug("Unpack failure", Err(err), "zone", dns.Zone(ctx))
			return
		}

		rd := r.RecursionDesired

		// (E)DNS compliance functions
		ctx = dnsctx.WithFunc(ctx, u,
			func(m *dns.Msg) *dns.Msg {
				m.RecursionAvailable = false // we are an authoritative server
				m.Zero = false
				m.RecursionDesired = rd
				return m
			})

		next.ServeDNS(ctx, w, r)
	})
}
