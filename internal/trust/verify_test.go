package trust

import (
	"context"
	"crypto"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
)

const zone = "windows.example."

var now = time.Unix(1793793600, 0)

func keyPair(t *testing.T) (*dns.DNSKEY, crypto.Signer) {
	t.Helper()
	key := dns.NewDNSKEY(zone, dns.ECDSAP256SHA256)
	key.Flags = 257
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	return key, private.(crypto.Signer)
}

func signed(t *testing.T, key *dns.DNSKEY, private crypto.Signer, set []dns.RR) []dns.RR {
	t.Helper()
	sig := dns.NewRRSIG(zone, key.Algorithm, key.KeyTag(), 1793793540, 1793793660)
	if err := sig.Sign(private, set, &dns.SignOption{}); err != nil {
		t.Fatal(err)
	}
	return append(set, sig)
}

func listener(t *testing.T, replies map[uint16][]dns.RR, change func(*dns.Msg)) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			b := make([]byte, 65535)
			n, address, err := conn.ReadFrom(b)
			if err != nil {
				return
			}
			q := &dns.Msg{Data: b[:n]}
			if q.Unpack() != nil {
				return
			}
			r := dns.NewMsg(zone, dns.RRToType(q.Question[0]))
			r.ID, r.Response, r.Authoritative = q.ID, true, true
			for _, rr := range replies[dns.RRToType(q.Question[0])] {
				r.Answer = append(r.Answer, rr.Clone())
			}
			if change != nil {
				change(r)
			}
			if r.Pack() != nil {
				return
			}
			if _, err := conn.WriteTo(r.Data, address); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = conn.Close(); <-done })
	return conn.LocalAddr().String()
}

func TestVerifyChecksExplicitInfrastructureTrust(t *testing.T) {
	key, private := keyPair(t)
	next, nextPrivate := keyPair(t)
	soa, err := dns.New(zone + " 60 IN SOA ns.example. hostmaster.example. 1 60 60 60 60")
	if err != nil {
		t.Fatal(err)
	}
	records := map[uint16][]dns.RR{dns.TypeDNSKEY: signed(t, key, private, []dns.RR{key, next}), dns.TypeSOA: signed(t, next, nextPrivate, []dns.RR{soa})}
	address := listener(t, records, nil)
	for _, anchor := range []dns.RR{key, key.ToDS(dns.SHA256)} {
		got := Verify(context.Background(), address, zone, anchor, now)
		if got.Status != "verified" || got.TrustSource != "explicit-local-anchor" {
			t.Fatalf("%+v", got)
		}
	}
	wrong, _ := keyPair(t)
	for _, anchor := range []dns.RR{wrong, wrong.ToDS(dns.SHA256)} {
		if got := Verify(context.Background(), address, zone, anchor, now); got.Status != "bogus" {
			t.Fatalf("%+v", got)
		}
	}
	for _, at := range []time.Time{now.Add(-61 * time.Second), now.Add(61 * time.Second)} {
		if got := Verify(context.Background(), address, zone, key, at); got.Status != "bogus" {
			t.Fatalf("%+v", got)
		}
	}
	for _, at := range []time.Time{now.Add(-60 * time.Second), now.Add(60 * time.Second)} {
		if got := Verify(context.Background(), address, zone, key, at); got.Status != "verified" {
			t.Fatalf("boundary %+v", got)
		}
	}
}

func TestVerifyReportsUnverifiedResponses(t *testing.T) {
	key, private := keyPair(t)
	cases := []struct {
		name, status string
		records      map[uint16][]dns.RR
		change       func(*dns.Msg)
	}{
		{"unsigned", "insecure", nil, nil},
		{"AD alone", "insecure", nil, func(r *dns.Msg) { r.AuthenticatedData = true }},
		{"missing SOA", "bogus", map[uint16][]dns.RR{dns.TypeDNSKEY: signed(t, key, private, []dns.RR{key})}, nil},
		{"unavailable", "indeterminate", nil, func(r *dns.Msg) { r.Rcode = dns.RcodeServerFailure }},
		{"not authoritative", "indeterminate", nil, func(r *dns.Msg) { r.Authoritative = false }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Verify(context.Background(), listener(t, c.records, c.change), zone, key, now)
			if got.Status != c.status {
				t.Fatalf("%+v", got)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := Verify(ctx, "127.0.0.1:1", zone, key, now); got.Status != "unreachable" {
		t.Fatalf("%+v", got)
	}
	if got := Verify(ctx, "127.0.0.1:1", zone, nil, now); got.Status != "indeterminate" || got.TrustSource != "none" {
		t.Fatalf("%+v", got)
	}
}

func TestParseAnchorSanitizesInvalidMaterial(t *testing.T) {
	key, _ := keyPair(t)
	for _, text := range []string{"SECRET PRIVATE MATERIAL", strings.Repeat("x", 4097), "elsewhere.example. IN TXT SECRET", strings.Replace(key.String(), " 13 ", " 99 ", 1)} {
		if _, err := ParseAnchor(text, zone); err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("unexpected error %v", err)
		}
	}
}

func TestParseAnchorRequiresOnePublicRecord(t *testing.T) {
	key, _ := keyPair(t)
	for _, text := range []string{"", "; comment", "$TTL 60", key.String() + "\n" + key.String()} {
		if _, err := ParseAnchor(text, zone); err == nil {
			t.Fatal("accepts input without one public record")
		}
	}
}

func TestSignatureRejectsUnsupportedSerialRanges(t *testing.T) {
	sig := dns.NewRRSIG(zone, dns.ECDSAP256SHA256, 1, 0, math.MaxUint32)
	sig.TypeCovered, sig.Labels = dns.TypeSOA, 2
	for _, bounds := range [][2]uint32{{0, math.MaxUint32}, {math.MaxUint32, 0}, {0, 1 << 31}} {
		sig.Inception, sig.Expiration = bounds[0], bounds[1]
		if validSignature(sig, dns.TypeSOA, zone, time.Unix(0, 0)) {
			t.Fatal("accepts unsupported serial interval")
		}
	}
	for _, bounds := range [][2]uint32{{0, 1}, {math.MaxUint32 - 1, math.MaxUint32}} {
		sig.Inception, sig.Expiration = bounds[0], bounds[1]
		if !validSignature(sig, dns.TypeSOA, zone, time.Unix(int64(bounds[0]), 0)) {
			t.Fatal("rejects supported boundary")
		}
	}
}

func TestVerifyRejectsMalformedInfrastructure(t *testing.T) {
	key, private := keyPair(t)
	good := signed(t, key, private, []dns.RR{key})
	for _, change := range []func([]dns.RR) []dns.RR{
		func(r []dns.RR) []dns.RR { r[0].Header().Name = "other.example."; return r },
		func(r []dns.RR) []dns.RR { r[0].Header().Class = dns.ClassCHAOS; return r },
		func(r []dns.RR) []dns.RR { r[1].(*dns.RRSIG).TypeCovered = dns.TypeTXT; return r },
		func(r []dns.RR) []dns.RR { r[1].(*dns.RRSIG).Labels = 1; return r },
		func(r []dns.RR) []dns.RR { r[1].(*dns.RRSIG).SignerName = "other.example."; return r },
		func(r []dns.RR) []dns.RR { r[1].(*dns.RRSIG).Signature = "AAAA"; return r },
		func(r []dns.RR) []dns.RR { rr, _ := dns.New(zone + " IN TXT bad"); return append(r, rr) },
		func(r []dns.RR) []dns.RR { return r[:1] },
	} {
		var records []dns.RR
		for _, rr := range good {
			records = append(records, rr.Clone())
		}
		if verifySet(change(records), dns.TypeDNSKEY, zone, []*dns.DNSKEY{key}, now) {
			t.Fatal("accepts malformed infrastructure")
		}
	}
	oversized := make([]dns.RR, 17)
	for i := range oversized {
		oversized[i], _ = dns.New(zone + " IN TXT x")
	}
	if verifySet(oversized, dns.TypeDNSKEY, zone, []*dns.DNSKEY{key}, now) {
		t.Fatal("accepts too many records")
	}
	got := Verify(context.Background(), listener(t, map[uint16][]dns.RR{dns.TypeDNSKEY: oversized}, nil), zone, key, now)
	if got.Status != "indeterminate" {
		t.Fatalf("%+v", got)
	}
	address := listener(t, map[uint16][]dns.RR{dns.TypeDNSKEY: good}, func(r *dns.Msg) {
		if dns.RRToType(r.Question[0]) == dns.TypeSOA {
			r.Rcode = dns.RcodeServerFailure
		}
	})
	if got := Verify(context.Background(), address, zone, key, now); got.Status != "indeterminate" {
		t.Fatalf("%+v", got)
	}
}

func TestVerifyRejectsInvalidTargets(t *testing.T) {
	key, _ := keyPair(t)
	for _, target := range [][2]string{{"bad", zone}, {"host.example:53", zone}, {"127.0.0.1:", zone}, {"127.0.0.1:53", "UPPER.example."}, {"127.0.0.1:53", "example"}, {"127.0.0.1:53", "."}, {"127.0.0.1:53", "x..example."}, {"127.0.0.1:53", "-x.example."}, {"127.0.0.1:53", "x-.example."}, {"127.0.0.1:53", "x_.example."}, {"127.0.0.1:53", strings.Repeat("x", 64) + ".example."}, {"127.0.0.1:53", strings.Repeat("x", 221)}} {
		if validTarget(target[0], target[1]) {
			t.Fatalf("accepts %q", target)
		}
	}
	if !validTarget("[::1]:1053", "w-1.example.") {
		t.Fatal("rejects IPv6")
	}
	for _, at := range []time.Time{time.Unix(-1, 0), time.Unix(math.MaxUint32+1, 0)} {
		if got := Verify(context.Background(), "127.0.0.1:53", zone, key, at); got.Status != "indeterminate" {
			t.Fatalf("%+v", got)
		}
	}
	invalid := key.Clone().(*dns.DNSKEY)
	invalid.Protocol = 0
	if got := Verify(context.Background(), "127.0.0.1:53", zone, invalid, now); got.Status != "indeterminate" {
		t.Fatalf("%+v", got)
	}
	if got := Verify(context.Background(), "bad", zone, key, now); got.Status != "indeterminate" {
		t.Fatalf("%+v", got)
	}
}

func TestVerifyRejectsInfrastructureFromANonZoneKey(t *testing.T) {
	for _, protocol := range []uint8{3, 2} {
		t.Run(string(rune('0'+protocol)), func(t *testing.T) {
			anchor, private := keyPair(t)
			other, otherPrivate := keyPair(t)
			if protocol == 3 {
				other.Flags = 0
			} else {
				other.Protocol = protocol
			}
			soa, err := dns.New(zone + " IN SOA ns.example. hostmaster.example. 1 60 60 60 60")
			if err != nil {
				t.Fatal(err)
			}
			records := map[uint16][]dns.RR{dns.TypeDNSKEY: signed(t, anchor, private, []dns.RR{anchor, other}), dns.TypeSOA: signed(t, other, otherPrivate, []dns.RR{soa})}
			result := Verify(context.Background(), listener(t, records, nil), zone, anchor, now)
			if result.Status != "bogus" {
				t.Fatalf("%+v", result)
			}
		})
	}
}
