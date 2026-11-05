package window

import (
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
)

type ordinarySigner interface {
	SignCurrentTXT(*dns.TXT, time.Time) (*dns.RRSIG, *dns.DNSKEY, error)
}

func TestOrdinarySignatureAcceptsTheFullDNSNameLimit(t *testing.T) {
	s := currentSigner(t)
	name := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 45) + ".windows.example."
	txt := &dns.TXT{Hdr: dns.Header{Name: name, Class: dns.ClassINET, TTL: 2147483647}, Txt: []string{"public"}}
	if len(name) != 254 {
		t.Fatal("fixture does not reach the DNS name limit")
	}
	if _, _, err := s.SignCurrentTXT(txt, testNow); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"a" + name, "bad_label.windows.example.", "event.windows.example"} {
		txt.Hdr.Name = bad
		if _, _, err := s.SignCurrentTXT(txt, testNow); err == nil {
			t.Fatalf("accepts invalid name %q", bad)
		}
	}
}

func currentSigner(t *testing.T) ordinarySigner {
	t.Helper()
	s, ok := any(testSigner(t)).(ordinarySigner)
	if !ok {
		t.Fatal("the configured key cannot sign an ordinary TXT record")
	}
	return s
}

func TestOrdinarySignatureUsesTheStaticOriginalTTL(t *testing.T) {
	s := currentSigner(t)
	txt := &dns.TXT{Hdr: dns.Header{Name: "event.windows.example.", Class: dns.ClassINET, TTL: 2147483647}, Txt: []string{"v=epochalypse1 event=2026-11-05T13:00:00Z"}}
	sig, key, err := s.SignCurrentTXT(txt, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if sig.OrigTTL != txt.Hdr.TTL || !sig.ValidPeriod(testNow) || !sig.ValidPeriod(testNow.Add(24*time.Hour)) {
		t.Fatalf("ordinary signature: %s", sig)
	}
	if err := sig.Verify(key, []dns.RR{txt}, &dns.SignOption{}); err != nil {
		t.Fatal(err)
	}
	key.Hdr.Name = "changed.example."
	_, nextKey, err := s.SignCurrentTXT(txt, testNow.Add(47*time.Hour))
	if err != nil || nextKey.Hdr.Name != "windows.example." {
		t.Fatalf("caller changes configured key: %v", err)
	}
	renewed, _, err := s.SignCurrentTXT(txt, testNow.Add(47*time.Hour))
	if err != nil || renewed.Inception == sig.Inception || !renewed.ValidPeriod(testNow.Add(71*time.Hour)) {
		t.Fatalf("ordinary signature does not renew: %v", err)
	}
}

func TestOrdinarySignatureRejectsForeignRecordsAndUnsupportedClocks(t *testing.T) {
	s := currentSigner(t)
	for _, name := range []string{"", "windows.example.", "event.other.example.", "event.notwindows.example.", "EVENT.windows.example."} {
		txt := &dns.TXT{Hdr: dns.Header{Name: name, Class: dns.ClassINET, TTL: 60}, Txt: []string{"public"}}
		if _, _, err := s.SignCurrentTXT(txt, testNow); err == nil {
			t.Fatalf("accepts foreign name %q", name)
		}
	}
	txt := &dns.TXT{Hdr: dns.Header{Name: "event.windows.example.", Class: dns.ClassINET, TTL: 60}, Txt: []string{"public"}}
	for _, now := range []time.Time{{}, time.Unix(299, 0), time.Unix(1<<32-1, 0)} {
		if _, _, err := s.SignCurrentTXT(txt, now); err == nil {
			t.Fatalf("accepts unsupported clock %v", now)
		}
	}
	if _, _, err := s.SignCurrentTXT(nil, testNow); err == nil {
		t.Fatal("accepts an absent TXT record")
	}
	txt.Hdr.Class = dns.ClassCHAOS
	if _, _, err := s.SignCurrentTXT(txt, testNow); err == nil {
		t.Fatal("accepts another DNS class")
	}
}
