package deleg_test

import (
	"bytes"
	"strings"
	"testing"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/deleg"
	"codeberg.org/miekg/dns/dnstest"
)

func TestDELEG(t *testing.T) {
	// TODO(miek): include more tests, or rename the test.
	testcases := []struct {
		in  string
		exp dns.RR
	}{
		{
			"$ORIGIN example.\nexample.   DELEG server-ipv4=192.0.2.1 server-ipv6=2001:DB8::1\n",
			dnstest.New("example. IN 3600  DELEG server-ipv4=192.0.2.1 server-ipv6=2001:DB8::1"),
		},
		{
			"$ORIGIN example.\nexample.   DELEG server-name=ns2,ns3.example.org.\n",
			dnstest.New("example. IN 3600  DELEG server-name=ns2.example.,ns3.example.org."),
		},
	}
	for i, tc := range testcases {
		zp := dns.NewZoneParser(strings.NewReader(tc.in), "", "")
		var rr dns.RR
		for r := range zp.All() {
			rr = r
			break
		}
		if err := zp.Err(); err != nil {
			t.Fatal(err)
		}
		if !dns.Equal(rr, tc.exp) {
			t.Errorf("test %d, expected %s, got %s", i, rr, tc.exp)
		}
	}
}

func TestDELEGPack(t *testing.T) {
	infos := []deleg.Info{&deleg.SERVERNAME{Hostnames: []string{"."}}, &deleg.INCLUDEDELEGPARAM{Domains: []string{"."}}}
	msg := make([]byte, 32)
	off, err := deleg.Pack(infos, msg, 0)
	if err != nil {
		t.Fatal(err)
	}
	exp := []byte{0, 3, 0, 1, 0, 0, 4, 0, 1, 0}
	if !bytes.Equal(msg[:off], exp) {
		t.Errorf("expected %v, got %v", exp, msg[:off])
	}
	// TODO(miek): Extend with more?
}
