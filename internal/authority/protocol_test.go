package authority

import (
	"context"
	"net"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/window"
	bolt "go.etcd.io/bbolt"
)

func TestAuthorityOmitsSignaturesWithoutDO(t *testing.T) {
	a, _, w := fixture(t)
	r := query(t, a, w.Name, dns.TypeTXT, false, true, now)
	if len(r.Answer) != 1 || r.Answer[0].String() != w.TXT {
		t.Fatal("non-DO response changes public TXT")
	}
	r = query(t, a, "absent."+w.Zone, dns.TypeTXT, false, false, now)
	if r.Rcode != dns.RcodeNameError || len(r.Ns) != 1 {
		t.Fatal("non-DO denial includes DNSSEC records")
	}
}

func TestAuthorityExposesOnlyOneTXTSignature(t *testing.T) {
	a, _, w := fixture(t)
	r := query(t, a, w.Name, dns.TypeRRSIG, false, false, now)
	txtCount := 0
	for _, rr := range r.Answer {
		sig := rr.(*dns.RRSIG)
		if sig.TypeCovered == dns.TypeTXT {
			txtCount++
			if sig.String() != w.RRSIG {
				t.Fatal("authority adds another TXT signature")
			}
		}
	}
	if txtCount != 1 {
		t.Fatal("RRSIG query does not expose the original TXT signature")
	}
}

func TestDenialProvesTheNameAndWildcardWithCurrentSignatures(t *testing.T) {
	a, _, w := fixture(t)
	for _, name := range []string{"zzzz." + w.Zone, "child." + w.Name} {
		r := query(t, a, name, dns.TypeTXT, true, false, now)
		if r.Rcode != dns.RcodeNameError {
			t.Fatal("missing name does not return NXDOMAIN")
		}
		for i := 0; i < len(r.Ns); i += 2 {
			sig := r.Ns[i+1].(*dns.RRSIG)
			if !sig.ValidPeriod(now) {
				t.Fatal("denial signature is stale")
			}
			if err := sig.Verify(a.Metadata().DNSKEY, []dns.RR{r.Ns[i]}, &dns.SignOption{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	r := query(t, a, w.Zone, dns.TypeDS, true, false, now)
	nsec := r.Ns[2].(*dns.NSEC)
	if nsec.Hdr.Name != w.Zone || slices.Contains(nsec.TypeBitMap, dns.TypeDS) {
		t.Fatal("child apex advertises a parental DS")
	}
	if !slices.Contains(nsec.TypeBitMap, dns.TypeDNSKEY) {
		t.Fatal("apex denial omits the DNSKEY type")
	}
	metadata := a.Metadata()
	metadata.DNSKEY.PublicKey = "changed"
	metadata.ParentDS.Digest = "changed"
	if a.Metadata().ParentDS.Digest == "changed" || a.Metadata().DNSKEY.PublicKey == "changed" {
		t.Fatal("metadata mutation changes authority state")
	}
}

func TestAuthorityRefreshesTheCacheAfterCreation(t *testing.T) {
	a, store, w := fixture(t)
	_ = query(t, a, w.Name, dns.TypeTXT, true, false, now)
	created, err := store.Create(window.Input{Content: "new public record", Inception: "2026-11-04T13:00:00Z", Expiration: "2026-11-04T14:00:00Z"}, now)
	if err != nil {
		t.Fatal(err)
	}
	r := query(t, a, created.Name, dns.TypeTXT, true, false, now)
	if len(r.Answer) != 2 || r.Answer[1].String() != created.RRSIG {
		t.Fatal("committed record is absent from the live cache")
	}
}

func TestAuthorityRefusesRecursiveAndUnsupportedQueries(t *testing.T) {
	a, _, w := fixture(t)
	for _, q := range []*dns.Msg{dns.NewMsg("outside.example.", dns.TypeTXT), dns.NewMsg(w.Zone, dns.TypeANY), dns.NewMsg(w.Zone, dns.TypeAXFR), dns.NewMsg(w.Zone, dns.TypeTXT, dns.ClassCHAOS), {}} {
		if err := q.Pack(); err != nil {
			t.Fatal(err)
		}
		r := &dns.Msg{Data: a.Packet(q.Data, false, now)}
		if err := r.Unpack(); err != nil {
			t.Fatal(err)
		}
		if r.Rcode != dns.RcodeRefused || r.RecursionAvailable || r.AuthenticatedData {
			t.Fatalf("unsupported request: %s", r)
		}
	}
	q := dns.NewMsg(w.Zone, dns.TypeTXT)
	q.Opcode = dns.OpcodeUpdate
	if err := q.Pack(); err != nil {
		t.Fatal(err)
	}
	r := &dns.Msg{Data: a.Packet(q.Data, false, now)}
	if err := r.Unpack(); err != nil {
		t.Fatal(err)
	}
	if r.Rcode != dns.RcodeRefused {
		t.Fatal("authority accepts DNS UPDATE")
	}
}

func TestAuthorityBoundsMalformedPackets(t *testing.T) {
	a, store, _ := fixture(t)
	if a.Packet(nil, true, now) != nil {
		t.Fatal("short request receives a response")
	}
	data := make([]byte, 12)
	data[2] = 0x80
	if a.Packet(data, true, now) != nil {
		t.Fatal("response packet receives another response")
	}
	data[2] = 0
	data[5] = 1
	r := &dns.Msg{Data: a.Packet(data, true, now)}
	if err := r.Unpack(); err != nil {
		t.Fatal(err)
	}
	if r.Rcode != dns.RcodeFormatError {
		t.Fatal("malformed question does not return FORMERR")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	r = query(t, a, "windows.example.", dns.TypeDNSKEY, true, false, now)
	if r.Rcode != dns.RcodeServerFailure || len(r.Answer) != 0 {
		t.Fatal("closed state returns an answer")
	}
}

func TestAuthorityRejectsAnUnsupportedInfrastructureClock(t *testing.T) {
	a, _, w := fixture(t)
	r := query(t, a, w.Zone, dns.TypeDNSKEY, true, false, time.Unix(-3600, 0))
	if r.Rcode != dns.RcodeServerFailure || len(r.Answer) != 0 {
		t.Fatal("unsupported infrastructure clock returns a signature")
	}
}

func TestAuthorityRejectsInvalidConfiguration(t *testing.T) {
	a, store, _ := fixture(t)
	if _, err := New(nil, a.signer, "ns.lab.invalid."); err == nil {
		t.Fatal("missing store passes")
	}
	if _, err := New(store, nil, "ns.lab.invalid."); err == nil {
		t.Fatal("missing signer passes")
	}
	for _, name := range []string{"", "ns", "ns.windows.example.", "NS.LAB.INVALID."} {
		if _, err := New(store, a.signer, name); err == nil {
			t.Fatal("invalid nameserver passes")
		}
	}
}

func TestAuthorityReturnsSERVFAILForCorruptState(t *testing.T) {
	state := t.TempDir()
	a, store, w := fixtureAt(t, state)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(filepath.Join(state, "v1", "windows.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error { return tx.Bucket([]byte("windows-v1")).Put([]byte(w.ID), []byte(`invalid`)) }); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := window.Open(state, a.signer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	a.store = reopened
	r := query(t, a, w.Name, dns.TypeTXT, true, false, now)
	if r.Rcode != dns.RcodeServerFailure || len(r.Answer) != 0 {
		t.Fatal("corrupt state serves public records")
	}
}

func TestQueryRejectsInvalidRequestsAndTransportFailures(t *testing.T) {
	for _, request := range []*dns.Msg{nil, {}} {
		if _, _, err := Query(context.Background(), "invalid", request); err == nil {
			t.Fatal("invalid question passes")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, network, err := Query(ctx, "127.0.0.1:1", dns.NewMsg("windows.example.", dns.TypeTXT)); err == nil || network != "udp" {
		t.Fatal("cancelled query returns success")
	}
}

func TestQueryReportsATCPFallbackFailure(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 65535)
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		response := append([]byte(nil), buf[:n]...)
		response[2] |= 0x82
		_, _ = conn.WriteTo(response, peer)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, network, err := Query(ctx, conn.LocalAddr().String(), dns.NewMsg("windows.example.", dns.TypeTXT)); err == nil || network != "tcp" {
		t.Fatal("failed TCP fallback returns success")
	}
	<-done
}

func TestAuthorityPreservesQueriesWithoutEDNS(t *testing.T) {
	a, _, w := fixture(t)
	q := dns.NewMsg(w.Name, dns.TypeTXT)
	if err := q.Pack(); err != nil {
		t.Fatal(err)
	}
	r := &dns.Msg{Data: a.Packet(q.Data, true, now)}
	if err := r.Unpack(); err != nil {
		t.Fatal(err)
	}
	if r.UDPSize != 0 || r.Security {
		t.Fatal("authority adds EDNS to a request without EDNS")
	}
}

func TestCanonicalNameOrderUsesTheRightmostLabelFirst(t *testing.T) {
	for _, pair := range [][2]string{{"windows.example.", "w0.windows.example."}, {"child.w0.windows.example.", "w1.windows.example."}, {"*.windows.example.", "w0.windows.example."}} {
		if compareName(pair[0], pair[1]) >= 0 {
			t.Fatal("NSEC order differs from canonical DNS order")
		}
	}
	if compareName("a.example.", "a.example.") != 0 || compareName("z.example.", "a.example.") <= 0 {
		t.Fatal("canonical comparison changes equal or ordered names")
	}
}
