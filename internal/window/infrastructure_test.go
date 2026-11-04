package window

import (
	"codeberg.org/miekg/dns"
	"testing"
	"time"
)

func TestInfrastructureSignatureKeepsTheDNSKEYCurrent(t *testing.T) {
	s := testSigner(t)
	key := s.PublicKey()
	sig, err := s.SignInfrastructure([]dns.RR{key}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if !sig.ValidPeriod(testNow) || !sig.ValidPeriod(testNow.Add(time.Hour)) {
		t.Fatal("infrastructure key expires during the refresh interval")
	}
	if err := sig.Verify(key, []dns.RR{key}, &dns.SignOption{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignInfrastructure([]dns.RR{makeTXT("window.windows.example.", "public")}, testNow); err == nil {
		t.Fatal("infrastructure signer adds an alternate TXT signature")
	}
}

func TestInfrastructureRejectsUnsupportedRecordsAndClocks(t *testing.T) {
	s := testSigner(t)
	key := s.PublicKey()
	for _, records := range [][]dns.RR{nil, {key, makeTXT("window.windows.example.", "public")}, {&dns.NS{Hdr: dns.Header{Name: "outside.example.", Class: dns.ClassINET}, Ns: "ns.lab.invalid."}}, {&dns.NS{Hdr: dns.Header{Name: "child.windows.example.", Class: dns.ClassINET}, Ns: "ns.lab.invalid."}}, {&dns.NS{Hdr: dns.Header{Name: "windows.example.", Class: dns.ClassCHAOS}, Ns: "ns.lab.invalid."}}} {
		if _, err := s.SignInfrastructure(records, testNow); err == nil {
			t.Fatal("unsupported infrastructure RRset passes")
		}
	}
	for _, at := range []time.Time{time.Unix(-1, 0), time.Unix(4294967296, 0)} {
		if _, err := s.SignInfrastructure([]dns.RR{key}, at); err == nil {
			t.Fatal("unsupported infrastructure clock passes")
		}
	}
	for _, at := range []time.Time{time.Unix(0, 0), time.Unix(4294967295, 0)} {
		sig, err := s.SignInfrastructure([]dns.RR{key}, at)
		if err != nil {
			t.Fatal(err)
		}
		if !sig.ValidPeriod(at) {
			t.Fatal("supported infrastructure clock loses validity")
		}
	}
}
