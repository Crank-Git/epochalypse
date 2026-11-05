package dbfile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnszone"
	"codeberg.org/miekg/dns/dnsutil"
)

// Transfer holds all the information to perform in incoming or outgoing zone transfer.
// The families from IPs, notifies and sources will be matched upon sending the actual notifies.
type Transfer struct {
	IPs []string

	TSIG       *dns.TSIG
	TSIGSecret string // base64

	Notifies []string
	Sources  []string
}

// Allow checks t.IPs against the response writer source address and returns nil if they match, otherwise
// an error is returned.
func (t *Transfer) Allow(w dns.ResponseWriter) error {
	if t == nil {
		return nil
	}

	if len(t.IPs) == 0 {
		return nil
	}

	src := dnsutil.RemoteIP(w)
	if src == "" {
		return fmt.Errorf("no remote IP found")
	}
	srcaddr, _ := netip.ParseAddr(src)
	srcaddr = srcaddr.Unmap()

	for _, ip := range t.IPs {
		addr, _ := netip.ParseAddr(ip)
		if addr.Unmap() == srcaddr {
			return nil
		}
	}

	return fmt.Errorf("no IPs match request: %s", srcaddr)
}

func (d *Dbfile) HandlerFuncTransfer(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, next dns.HandlerFunc) {
	z := d.Zone(dns.Zone(ctx))

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
	alog := log().With(slog.String("zone", z.Origin()), slog.String("file", filepath.Base(z.Path)), slog.Any("upstream", w.RemoteAddr()), slog.Uint64("serial", uint64(dnszone.Serial(z))))
	alog.Info("Successful transfer out")
}

func (d *Dbfile) TransferIn(origin string) error {
	// Save into temp file and then move this file over the dbfile path.
	// We also create a .dbfile.transferred empty file to indicate this file was fetched.
	c := dns.NewClient()
	m := dns.NewMsg(origin, dns.TypeAXFR)

	f, err := os.CreateTemp(filepath.Dir(d.Path), "xxxxx.transferred")
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(f.Name())

	var (
		terr error
		soa  int
	)

	alog := log().With(slog.String("zone", origin), slog.String("path", filepath.Base(d.Path)))

Transfer:
	for _, ip := range d.From.IPs {
		soa = 0
		env, err := c.TransferIn(context.TODO(), m, ip, nil)
		terr = err
		if err != nil {
			continue Transfer
		}
		for e := range env {
			if e.Error != nil {
				alog.Warn("Failed to transfer in", Err(err))
				terr = e.Error
				continue Transfer
			}
			for _, rr := range e.Answer {
				if _, ok := rr.(*dns.SOA); ok {
					soa++
					if soa > 1 {
						continue
					}
				}
				if !dnsutil.IsName(rr.Header().Name) {
					alog.With(slog.String("name", rr.Header().Name)).Debug("Invaild owner name")
					continue
				}

				io.WriteString(f, rr.String())
				f.Write([]byte("\n"))
			}
		}
		break
	}
	if terr != nil {
		return fmt.Errorf("failed to transfer in: %w", terr)
	}
	if soa != 2 {
		return fmt.Errorf("failed to transfer in: did not get start/end SOA record")
	}

	f.Close()
	alog.Info("Successful transfer in")
	os.WriteFile(transferpath(d.Path), nil, 0600)
	return os.Rename(f.Name(), d.Path)
}

// IsTransferred returns the path name of the sentinel file if the zone file was transferred from the internet. This is done by checking
// if .transferred_<path> exists. If it doees not exist the empty string is returned.
func (d *Dbfile) IsTransferred() string {
	p := transferpath(d.Path)
	_, err := os.Stat(p)
	if !errors.Is(err, os.ErrNotExist) {
		return p
	}
	return ""
}

func transferpath(s string) string {
	return filepath.Join(filepath.Dir(s), ".transferred_"+filepath.Base(s))
}
