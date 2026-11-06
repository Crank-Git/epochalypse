package authority

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/installation"
)

func countdownAuthorityFixture(t *testing.T) (*Authority, *countdown.Store) {
	t.Helper()
	a := &Authority{zone: "countdown.example.", nameserver: "ns.lab.invalid."}
	store, err := countdown.Open(t.TempDir(), installation.DefaultLimits(), a.zone)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	a.countdowns = store
	a.countdownSigner = countdownSignerFixture(t, a.zone)
	a.key = a.countdownSigner
	return a, store
}

func TestCountdownAuthorityUsesPreciseRemainingSeconds(t *testing.T) {
	a, store := countdownAuthorityFixture(t)
	record, err := store.Create(context.Background(), countdown.Input{Title: "Synthetic event", EventAt: now.Add(time.Hour).Format(time.RFC3339)}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		at   time.Time
		ttl  uint32
	}{
		{"exact", now, 3600},
		{"subsecond", now.Add(time.Nanosecond), 3599},
		{"between", now.Add(1500 * time.Millisecond), 3598},
		{"last fraction", record.EventAt.Add(-time.Nanosecond), 0},
		{"event", record.EventAt, 0},
		{"renew after event", record.EventAt.Add(72 * time.Hour), 0},
		{"rollback", now.Add(-time.Hour), 7200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, udp := range []bool{true, false} {
				reply := query(t, a, record.Name, dns.TypeTXT, true, udp, tc.at)
				if reply.Rcode != dns.RcodeSuccess || len(reply.Answer) != 2 {
					t.Fatalf("countdown response: %s", reply)
				}
				txt, sig := reply.Answer[0].(*dns.TXT), reply.Answer[1].(*dns.RRSIG)
				if txt.Hdr.TTL != tc.ttl || sig.Hdr.TTL != tc.ttl {
					t.Fatalf("TTL=%d/%d want %d", txt.Hdr.TTL, sig.Hdr.TTL, tc.ttl)
				}
				if txt.Txt[0] != record.TXT().Txt[0] || sig.OrigTTL != countdown.MaxTTL || !sig.ValidPeriod(tc.at) {
					t.Fatal("event or signature validity changes")
				}
				if err := sig.Verify(a.key.PublicKey(), []dns.RR{txt}, &dns.SignOption{}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	retained, err := store.Get(record.ID)
	if err != nil || retained != record {
		t.Fatalf("retained event changes: %v", err)
	}
}

func TestCountdownAuthorityCapsHorizonAfterClockRollback(t *testing.T) {
	a, store := countdownAuthorityFixture(t)
	horizon, err := store.Create(context.Background(), countdown.Input{Title: "Horizon event", EventAt: now.Add(countdown.MaxTTL * time.Second).Format(time.RFC3339)}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{now, now.Add(-time.Second)} {
		reply := query(t, a, horizon.Name, dns.TypeTXT, true, true, at)
		if len(reply.Answer) != 2 || reply.Answer[0].Header().TTL != countdown.MaxTTL || reply.Answer[1].(*dns.RRSIG).OrigTTL != countdown.MaxTTL {
			t.Fatalf("horizon or rollback exceeds the signed TTL: %s", reply)
		}
	}
}

func countdownSignerFixture(t *testing.T, zone string) *countdown.Signer {
	t.Helper()
	key := dns.NewDNSKEY(zone, dns.ECDSAP256SHA256)
	key.Flags = 257
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic.private")
	if err := os.WriteFile(path, []byte(key.PrivateKeyString(private)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path[:len(path)-8]+".key", []byte(key.String()), 0600); err != nil {
		t.Fatal(err)
	}
	signer, err := countdown.LoadSigner(zone, path)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}
