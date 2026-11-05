package authority

import (
	"codeberg.org/miekg/dns"
	"strings"
	"testing"
)

func rawQuestion(labels ...[]byte) []byte {
	data := []byte{0x42, 0x42, 1, 0, 0, 1, 0, 0, 0, 0, 0, 1}
	for _, label := range labels {
		if len(label) > 63 {
			panic("test label exceeds the DNS wire limit")
		}
		data = append(data, byte(len(label))) // #nosec G115 -- The label length is bounded to 63 bytes above.
		data = append(data, label...)
	}
	data = append(data, 0, 0, 16, 0, 1)
	return append(data, 0, 0, 41, 4, 208, 0, 0, 128, 0, 0, 0)
}

func TestAuthorityRefusesWireLabelsBeforeLossyDecode(t *testing.T) {
	a, _, w := fixture(t)
	for name, labels := range map[string][][]byte{
		"literal dot":              {[]byte("z.a"), []byte("windows"), []byte("example")},
		"full window in one label": {[]byte(strings.TrimSuffix(w.Name, "."))},
		"backslash":                {[]byte(`z\a`), []byte("windows"), []byte("example")},
		"control":                  {{'z', 0, 'a'}, []byte("windows"), []byte("example")},
		"non-ASCII":                {{'z', 255, 'a'}, []byte("windows"), []byte("example")},
	} {
		t.Run(name, func(t *testing.T) {
			r := &dns.Msg{Data: a.Packet(rawQuestion(labels...), true, now)}
			if err := r.Unpack(); err != nil {
				t.Fatal(err)
			}
			if r.Rcode != dns.RcodeRefused || r.Authoritative || len(r.Answer) != 0 || len(r.Ns) != 0 {
				t.Fatalf("unsupported raw name receives an answer or proof: %s", r)
			}
		})
	}
}

func TestAuthorityBoundsCompressedAndMalformedRawNames(t *testing.T) {
	a, _, _ := fixture(t)
	for _, payload := range [][]byte{{0xc0}, {0xc0, 255, 0, 16, 0, 1}, {0xc0, 12, 0, 16, 0, 1}, {64, 0}, {10, 'a'}, {0}} {
		data := append([]byte{0x42, 0x42, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}, payload...)
		r := &dns.Msg{Data: a.Packet(data, true, now)}
		if err := r.Unpack(); err != nil {
			t.Fatal(err)
		}
		if r.Rcode != dns.RcodeFormatError || r.Authoritative || len(r.Answer) != 0 || len(r.Ns) != 0 {
			t.Fatal("malformed raw name receives authority evidence")
		}
	}
	large := rawQuestion([]byte(strings.Repeat("a", 63)), []byte(strings.Repeat("b", 63)), []byte(strings.Repeat("c", 63)), []byte(strings.Repeat("d", 63)))
	r := &dns.Msg{Data: a.Packet(large, true, now)}
	if err := r.Unpack(); err != nil {
		t.Fatal(err)
	}
	if r.Rcode != dns.RcodeFormatError {
		t.Fatal("oversized name passes the wire guard")
	}
}

func TestAuthorityChecksOctetsBehindACompressionPointer(t *testing.T) {
	a, _, _ := fixture(t)
	data := []byte{0x42, 0x42, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0xc0, 18, 0, 16, 0, 1, 3, 'z', '.', 'a', 7, 'w', 'i', 'n', 'd', 'o', 'w', 's', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0}
	r := &dns.Msg{Data: a.Packet(data, true, now)}
	if err := r.Unpack(); err != nil {
		t.Fatal(err)
	}
	if r.Rcode != dns.RcodeRefused || r.Authoritative || len(r.Ns) != 0 {
		t.Fatal("compression bypasses the raw-octet guard")
	}
}
