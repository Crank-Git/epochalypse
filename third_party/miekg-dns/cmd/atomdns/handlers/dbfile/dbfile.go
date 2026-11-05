package dbfile

import (
	"context"
	"io"
	"sync"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/handlers/dbfile/zone"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnsctx"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnslog"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnszone"
	"codeberg.org/miekg/dns/dnsutil"
)

type Dbfile struct {
	Path string

	// Zones holds all the zone this instance of Dbfile is called for.
	Zones        map[string]*zone.Zone
	sync.RWMutex // protects Zones

	ctx    context.Context
	cancel context.CancelFunc

	To   *Transfer
	From *Transfer
}

func (d *Dbfile) HandlerFunc(next dns.HandlerFunc) dns.HandlerFunc {
	return dns.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
		if r.Opcode == dns.OpcodeNotify {
			d.HandlerFuncNotify(ctx, w, r)
			return
		}

		if dnsutil.IsTransfer(r) > 0 {
			d.HandlerFuncTransfer(ctx, w, r, next)
			return
		}

		z := d.Zone(dns.Zone(ctx))
		if z == nil {
			next.ServeDNS(ctx, w, r) // call refused guard
			return
		}

		m := dnszone.Retrieve(z, r, nil)
		if m == nil { // call refused guard
			next.ServeDNS(ctx, w, r)
			return
		}

		m.Data = r.Data
		m = dnsctx.Funcs(ctx, m)
		if err := m.Pack(); err != nil {
			dnslog.PackFail(ctx, log(), Err(err))
			return
		}
		io.Copy(w, m)
	})
}

func (d *Dbfile) Zone(origin string) *zone.Zone {
	d.RLock()
	z := d.Zones[origin]
	d.RUnlock()
	return z
}
