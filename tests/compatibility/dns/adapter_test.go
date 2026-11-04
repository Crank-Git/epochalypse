package compatibility_test

import (
	"crypto"
	"testing"
	"time"

	modern "codeberg.org/miekg/dns"
	legacy "github.com/miekg/dns"
)

func TestSuccessorVerifiesLegacySignature(t *testing.T) {
	key := &legacy.DNSKEY{Hdr: legacy.RR_Header{Name: "demo.example.", Rrtype: legacy.TypeDNSKEY, Class: legacy.ClassINET, Ttl: 60}, Flags: 257, Protocol: 3, Algorithm: legacy.ECDSAP256SHA256}
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	txt, err := legacy.NewRR(`window.demo.example. 60 IN TXT "public synthetic content"`)
	if err != nil {
		t.Fatal(err)
	}
	inception := uint32(time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC).Unix())
	sig := &legacy.RRSIG{Hdr: legacy.RR_Header{Name: "window.demo.example.", Rrtype: legacy.TypeRRSIG, Class: legacy.ClassINET, Ttl: 60}, Algorithm: key.Algorithm, KeyTag: key.KeyTag(), SignerName: key.Hdr.Name, Inception: inception, Expiration: inception + 3600}
	if err := sig.Sign(private.(crypto.Signer), []legacy.RR{txt}); err != nil {
		t.Fatal(err)
	}
	newKey := parseModern(t, key.String()).(*modern.DNSKEY)
	newTXT := parseModern(t, txt.String())
	newSig := parseModern(t, sig.String()).(*modern.RRSIG)
	if err := newSig.Verify(newKey, []modern.RR{newTXT}, &modern.SignOption{}); err != nil {
		t.Fatal(err)
	}
	assertPeriod(t, newSig, inception)
	if err := newSig.Sign(private.(crypto.Signer), []modern.RR{newTXT}, &modern.SignOption{}); err != nil {
		t.Fatal(err)
	}
	reverse, err := legacy.NewRR(newSig.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := reverse.(*legacy.RRSIG).Verify(key, []legacy.RR{txt}); err != nil {
		t.Fatal(err)
	}
	if newKey.ToDS(modern.SHA256).Digest != key.ToDS(legacy.SHA256).Digest {
		t.Fatal("adapters produce different trust anchors")
	}
	changed := parseModern(t, `window.demo.example. 60 IN TXT "changed content"`)
	if err := newSig.Verify(newKey, []modern.RR{changed}, &modern.SignOption{}); err == nil {
		t.Fatal("successor accepts changed content")
	}
}

func parseModern(t *testing.T, text string) modern.RR {
	t.Helper()
	rr, err := modern.New(text)
	if err != nil {
		t.Fatal(err)
	}
	return rr
}

func assertPeriod(t *testing.T, sig *modern.RRSIG, inception uint32) {
	t.Helper()
	for _, boundary := range []struct {
		offset int64
		valid  bool
	}{{-1, false}, {0, true}, {1, true}, {3599, true}, {3600, true}, {3601, false}} {
		if got := sig.ValidPeriod(time.Unix(int64(inception)+boundary.offset, 0)); got != boundary.valid {
			t.Errorf("offset %d: validity = %v, want %v", boundary.offset, got, boundary.valid)
		}
	}
}
