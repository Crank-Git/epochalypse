package random

import (
	"context"
	"math/rand/v2"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnsctx"
)

type Random struct{}

func (ra *Random) HandlerFunc(next dns.HandlerFunc) dns.HandlerFunc {
	return dns.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
		if r.Rcode != dns.RcodeSuccess {
			next.ServeDNS(ctx, w, r)
			return
		}
		switch r.Question[0].(type) {
		case *dns.ANY:
			next.ServeDNS(ctx, w, r)
			return
		case *dns.AXFR:
			next.ServeDNS(ctx, w, r)
			return
		case *dns.IXFR:
			next.ServeDNS(ctx, w, r)
			return
		}

		ctx = dnsctx.WithFunc(ctx, ra,
			func(m *dns.Msg) *dns.Msg {
				m.Answer = random(m.Answer)
				return m
			})

		next.ServeDNS(ctx, w, r)
	})
}

func random(in []dns.RR) []dns.RR {
	cname := []dns.RR{}
	address := []dns.RR{}
	rest := []dns.RR{}
	for _, r := range in {
		switch r.(type) {
		case *dns.CNAME:
			cname = append(cname, r)
		case *dns.A, *dns.AAAA:
			address = append(address, r)
		default:
			rest = append(rest, r)
		}
	}

	rand.Shuffle(len(address), func(i, j int) {
		address[i], address[j] = address[j], address[i]
	})

	out := append(cname, rest...)
	out = append(out, address...)
	return out
}
