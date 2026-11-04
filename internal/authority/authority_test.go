package authority

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/window"
)

var now = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func fixture(t *testing.T) (*Authority, *window.Store, window.Window) {
	return fixtureAt(t, t.TempDir())
}

func fixtureAt(t *testing.T, state string) (*Authority, *window.Store, window.Window) {
	t.Helper()
	key := dns.NewDNSKEY("windows.example.", dns.ECDSAP256SHA256)
	key.Flags = 257
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic.private")
	for path, text := range map[string]string{path: key.PrivateKeyString(private), strings.TrimSuffix(path, ".private") + ".key": key.String()} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	signer, err := window.LoadSigner("windows.example.", path)
	if err != nil {
		t.Fatal(err)
	}
	store, err := window.Open(state, signer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	w, err := store.Create(window.Input{Content: "public authority experiment", Inception: "2026-11-04T13:00:00Z", Expiration: "2026-11-04T14:00:00Z"}, now)
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(store, signer, "ns.lab.invalid.")
	if err != nil {
		t.Fatal(err)
	}
	return a, store, w
}

func query(t *testing.T, a *Authority, name string, typ uint16, secure, udp bool, at time.Time) *dns.Msg {
	t.Helper()
	m := dns.NewMsg(name, typ)
	m.Security = secure
	m.UDPSize = 1232
	if err := m.Pack(); err != nil {
		t.Fatal(err)
	}
	r := &dns.Msg{Data: a.Packet(m.Data, udp, at)}
	if err := r.Unpack(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAuthorityPublishesTheStoredFutureSignature(t *testing.T) {
	a, _, w := fixture(t)
	r := query(t, a, w.Name, dns.TypeTXT, true, true, now)
	if r.Rcode != dns.RcodeSuccess || !r.Authoritative || r.RecursionAvailable || len(r.Answer) != 2 {
		t.Fatalf("future TXT response: %s", r)
	}
	if r.Answer[0].String() != w.TXT || r.Answer[1].String() != w.RRSIG {
		t.Fatal("authority changes the stored records")
	}
	if r.Answer[1].(*dns.RRSIG).ValidPeriod(now) {
		t.Fatal("authority adds a valid TXT signature")
	}
	late := query(t, a, w.Name, dns.TypeTXT, true, false, w.Expiration.Add(time.Second))
	if len(late.Answer) != 2 || late.Answer[1].String() != w.RRSIG {
		t.Fatal("authority replaces the expired signature")
	}
}

func TestAuthorityKeepsInfrastructureSignaturesValid(t *testing.T) {
	a, _, w := fixture(t)
	for _, typ := range []uint16{dns.TypeDNSKEY, dns.TypeNS, dns.TypeSOA} {
		r := query(t, a, w.Zone, typ, true, false, now.Add(48*time.Hour))
		if r.Rcode != dns.RcodeSuccess || len(r.Answer) != 2 {
			t.Fatalf("infrastructure response: %s", r)
		}
		if !r.Answer[1].(*dns.RRSIG).ValidPeriod(now.Add(48 * time.Hour)) {
			t.Fatal("infrastructure signature is stale")
		}
	}
}

func TestAuthoritySignsDenialWithoutAChildApexDS(t *testing.T) {
	a, _, w := fixture(t)
	for _, tc := range []struct {
		name  string
		typ   uint16
		rcode uint16
	}{
		{"absent." + w.Zone, dns.TypeTXT, dns.RcodeNameError},
		{w.Name, dns.TypeAAAA, dns.RcodeSuccess},
		{w.Zone, dns.TypeDS, dns.RcodeSuccess},
	} {
		r := query(t, a, tc.name, tc.typ, true, false, now)
		if r.Rcode != tc.rcode || len(r.Answer) != 0 || len(r.Ns) < 4 {
			t.Fatalf("denial response: %s", r)
		}
		found := false
		for _, rr := range r.Ns {
			if _, ok := rr.(*dns.NSEC); ok {
				found = true
			}
		}
		if !found {
			t.Fatal("denial omits NSEC")
		}
	}
}
