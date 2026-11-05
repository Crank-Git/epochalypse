package panic

import (
	"context"
	"log/slog"

	"codeberg.org/miekg/dns"
)

type Panic int

func (p *Panic) HandlerFunc(next dns.HandlerFunc) dns.HandlerFunc {
	return dns.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
		defer func() {
			if r := recover(); r != nil {
				Panics.Inc()
				slog.Error("Recovered from panic", "panic", r)
			}
		}()
		next.ServeDNS(ctx, w, r)
	})
}
