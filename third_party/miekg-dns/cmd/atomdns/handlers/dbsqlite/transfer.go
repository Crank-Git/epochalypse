package dbsqlite

import (
	"context"
	"log/slog"
	"path/filepath"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnszone"
	"codeberg.org/miekg/dns/dnsutil"
)

func (d *Dbsqlite) HandlerFuncTransfer(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, next dns.HandlerFunc) {
	z := d.Zones[dns.Zone(ctx)]

	if z == nil || dnsutil.Network(w) == "udp" {
		next.ServeDNS(ctx, w, r)
		return
	}

	if err := d.To.Allow(w); err != nil {
		log().Debug("Transfer not allowed", Err(err))
		next.ServeDNS(ctx, w, r)
		return
	}

	if err := dnszone.TransferOut(z, ctx, w, r); err != nil {
		log().Debug("Failure to transfer out", Err(err))
		return
	}
	alog := log().With(slog.String("zone", z.Origin()), slog.String("path", filepath.Base(d.Path)), slog.Any("upstream", w.RemoteAddr()), slog.Uint64("serial", uint64(dnszone.Serial(z))))
	alog.Info("Successful transfer out")
}
