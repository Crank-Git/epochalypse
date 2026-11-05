//go:build resolvers

package resolvers_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/authority"
	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/installation"
)

func createCountdownFixture(t *testing.T, directory string) map[string]countdown.Record {
	t.Helper()
	store, err := countdown.Open(filepath.Join(directory, "countdown-state"), installation.DefaultLimits(), "windows.example.")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	now := time.Now().UTC().Truncate(time.Second)
	records := map[string]countdown.Record{}
	for label, event := range map[string]time.Time{"active": now.Add(time.Hour), "capped": now.Add(365 * 24 * time.Hour), "complete": now.Add(-time.Hour)} {
		record, err := store.Create(context.Background(), countdown.Input{Title: "Synthetic countdown", EventAt: event.Format(time.RFC3339)}, now.Add(-2*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		records[label] = record
	}
	return records
}

func openCountdownAuthority(t *testing.T) *authority.Authority {
	t.Helper()
	// The synthetic lab shares its trust anchor to isolate the TTL validation rule.
	signer, err := countdown.LoadSigner("windows.example.", "/fixture/synthetic.private")
	if err != nil {
		t.Fatal(err)
	}
	store, err := countdown.Open("/fixture/countdown-state", installation.DefaultLimits(), "windows.example.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	a, err := authority.NewCountdown(store, signer, "ns.lab.invalid.")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func countdownResponse(t *testing.T, address string, record countdown.Record, authoritative bool) (*dns.TXT, *dns.RRSIG) {
	t.Helper()
	request := dns.NewMsg(record.Name, dns.TypeTXT)
	request.Security = true
	reply, _, err := authority.Query(context.Background(), address, request)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Rcode != dns.RcodeSuccess || len(reply.Answer) != 2 {
		t.Fatalf("countdown answer: %s", reply)
	}
	txt, sig := reply.Answer[0].(*dns.TXT), reply.Answer[1].(*dns.RRSIG)
	if txt.Txt[0] != record.TXT().Txt[0] || sig.OrigTTL != countdown.MaxTTL || sig.KeyTag != 0 {
		t.Fatalf("absolute event or Original TTL changes: %s", reply)
	}
	if authoritative && !reply.Authoritative {
		t.Fatalf("authority omits AA: %s", reply)
	}
	if !authoritative && !reply.AuthenticatedData {
		t.Fatalf("resolver rejects DNSSEC: %s", reply)
	}
	return txt, sig
}

func TestCountdownResolversValidateLoweredTTL(t *testing.T) {
	lab := newResolverLab(t)
	for _, kind := range []string{"unbound-strict", "unbound-default", "bind"} {
		target := lab.startResolver(t, kind, kind+"-countdown")
		for label, record := range lab.countdowns {
			before := time.Now()
			authoritative, _ := countdownResponse(t, lab.authorityAddress, record, true)
			after := time.Now()
			if authoritative.Hdr.TTL > record.Timing(before).AuthoritativeTTL || authoritative.Hdr.TTL < record.Timing(after).AuthoritativeTTL {
				t.Fatal("authority omits the precise query clock")
			}
			txt, sig := countdownResponse(t, target.Address, record, false)
			if txt.Hdr.TTL > authoritative.Hdr.TTL || txt.Hdr.TTL > sig.OrigTTL || txt.Hdr.TTL > uint32(time.Until(time.Unix(int64(sig.Expiration), 0))/time.Second)+1 {
				t.Fatal("recursive TTL exceeds the validation bounds")
			}
			if label == "complete" && txt.Hdr.TTL != 0 {
				t.Fatal("completed event loses TTL zero")
			}
			if label == "capped" && txt.Hdr.TTL >= authoritative.Hdr.TTL {
				t.Fatal("resolver omits its signature expiration or TTL cap")
			}
			t.Logf("%s %s authoritative=%d recursive=%d original=%d event=%s", kind, label, authoritative.Hdr.TTL, txt.Hdr.TTL, sig.OrigTTL, record.EventAt.Format(time.RFC3339))
		}
	}
}
