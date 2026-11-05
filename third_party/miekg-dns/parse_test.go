package dns

import (
	"fmt"
	"testing"
	"time"

	"codeberg.org/miekg/dns/deleg"
	"codeberg.org/miekg/dns/internal/dnsfuzz"
	"codeberg.org/miekg/dns/svcb"
)

func TestNew(t *testing.T) {
	testcases := []struct {
		name string
		in   string
		fn   func(RR) error
		err  bool
	}{
		{
			"SVCB/ALPN", `. 1 IN SVCB 10 one.test. alpn=h2`,
			func(rr RR) error {
				alpn := rr.(*SVCB).Value[0].(*svcb.ALPN).Alpn
				if alpn[0] != "h2" {
					return fmt.Errorf("wanted %v got %v", "h2", alpn)
				}
				return nil
			},
			false,
		},
		{
			"SVCB/ALPN", `. 2 IN SVCB 20 two.test. alpn=h2,h3-19`,
			func(rr RR) error {
				v := []string{"h2", "h3-19"}
				alpn := rr.(*SVCB).Value[0].(*svcb.ALPN).Alpn
				for i := range v {
					if v[i] != alpn[i] {
						return fmt.Errorf("wanted %v got %v", v, alpn)
					}
				}
				return nil
			},
			false,
		},
		{
			"SVCB/ALPN", `. 5 IN SVCB 50 five.test. alpn=part1\,\p\a\r\t2\044part3\092,part4\092\\`,
			func(rr RR) error {
				v := []string{"part1", "part2", `part3,part4\`}
				alpn := rr.(*SVCB).Value[0].(*svcb.ALPN).Alpn
				for i := range v {
					if v[i] != alpn[i] {
						return fmt.Errorf("wanted %v got %v", v, alpn)
					}
				}
				return nil
			},
			false,
		},
		{
			"HTTPS", `alice.example.org. HTTPS 1 `,
			func(rr RR) error {
				if rr != nil {
					return fmt.Errorf("expected error and empty RR, got %v", rr)
				}
				return nil
			},
			true,
		},
		{
			"HTTPS", `alice.example.org. HTTPS 1\"x.\nbob.example.org. 300 IN A 192.0.2.66\nalice.example.org. HTTPS 1`,
			func(rr RR) error {
				if rr != nil {
					return fmt.Errorf("expected error and empty RR, got %v", rr)
				}
				return nil
			},
			true,
		},
		{
			"DSYNC", `child._dsync.example. IN DSYNC CDS NOTIFY 5300 rr-endpoint.example.`,
			func(rr RR) error {
				dsync := rr.(*DSYNC)
				if dsync.Scheme != 1 {
					return fmt.Errorf("expected scheme 1, got %d", dsync.Scheme)
				}
				if dsync.Port != 5300 {
					return fmt.Errorf("expected port 5300, got %d", dsync.Port)
				}
				if dsync.Target != "rr-endpoint.example." {
					return fmt.Errorf("expected port rr-endpoint.example., got %s", "rr-endpoint.example.")
				}
				return nil
			},
			false,
		},
		{
			"DELEG", "example.org. IN DELEG server-ipv4=192.0.2.1 server-ipv6=2001:DB8::1",
			func(rr RR) error {
				dlg := rr.(*DELEG)
				v0 := dlg.Value[0]
				v1 := dlg.Value[1]
				_ = v0.(*deleg.SERVERIPV4)
				_ = v1.(*deleg.SERVERIPV6)
				return nil
			},
			false,
		},
		{
			"DELEG", `example.org. IN DELEG server-ipv4="192.0.2.1" server-ipv6="2001:DB8::1"`,
			func(rr RR) error {
				dlg := rr.(*DELEG)
				v0 := dlg.Value[0]
				v1 := dlg.Value[1]
				_ = v0.(*deleg.SERVERIPV4)
				_ = v1.(*deleg.SERVERIPV6)
				return nil
			},
			false,
		},
		{
			"A no data", `www.example.org. IN A`,
			func(rr RR) error {
				if rr != nil {
					return fmt.Errorf("expected error and empty RR, got %v", rr)
				}
				return nil
			},
			true,
		},
		{
			"NSEC3",
			"k36vo59bkum4osckkrd8tvibdgr0njbc.nl. 599 IN NSEC3 1 0 0 - K36VONMLM2T8IF3G8P5AV864OHLTB7K7 NS SOA TXT RRSIG DNSKEY NSEC3PARAM",
			func(rr RR) error {
				nsec3 := rr.(*NSEC3)
				if x := nsec3.NextDomain; x != "K36VONMLM2T8IF3G8P5AV864OHLTB7K7" {
					return fmt.Errorf("expected %s, got %s", x, nsec3.NextDomain)
				}
				if x := nsec3.TypeBitMap[0]; x != TypeNS {
					return fmt.Errorf("expected %s, got %s", TypeToString[x], TypeToString[nsec3.TypeBitMap[0]])
				}
				if x := nsec3.TypeBitMap[5]; x != TypeNSEC3PARAM {
					return fmt.Errorf("expected %s, got %s", TypeToString[TypeNSEC3PARAM], TypeToString[nsec3.TypeBitMap[4]])
				}
				return nil
			},
			false,
		},

		{
			"LOC",
			"SW1A2AA.find.me.uk.	LOC	51 30 12.748 N 00 07 39.611 W 0.00m 0.00m 0.00m 0.00m",
			func(rr RR) error {
				// TODO(miek)
				return nil
			},
			false,
		},
		{
			"DS",
			"0-0-1.se.               3600    IN      DS      12412 8 2 47783E3806F62788EF4E4C69D1AFE48262BEC34872E8C400132107A7 6D442D82",
			func(rr RR) error {
				ds := rr.(*DS)
				if x := ds.KeyTag; x != 12412 {
					return fmt.Errorf("expected %d, got %d", 12412, x)
				}
				return nil
			},
			false,
		},
		{
			"RFC3597",
			`example.com. 3600 IN TYPE65280 \# 4 0A000001`,
			func(rr RR) error {
				exp := "example.com.\t3600\tIN\tTYPE65280\t\\# 4 0A000001"
				if x := rr.String(); x != exp {
					return fmt.Errorf("expected %q, got %q", exp, x)
				}
				return nil
			},
			false,
		},
		{
			"IPN",
			`ipn.example.org. 3600 IN IPN 18446744073709551615`,
			func(rr RR) error {
				exp := "ipn.example.org.\t3600\tIN\tIPN\t18446744073709551615"
				if x := rr.String(); x != exp {
					return fmt.Errorf("expected %q, got %q", exp, x)
				}
				return nil
			},
			false,
		},
		{
			"CERT",
			`cert.test. CERT PKIX 0 0 Y2VydGlmaWNhdGUtZGF0YQ==`,
			func(rr RR) error {
				cert := rr.(*CERT)
				if cert.Type != 1 {
					return fmt.Errorf("expected type 1, got %d", cert.Type)
				}
				if cert.Certificate != "Y2VydGlmaWNhdGUtZGF0YQ==" {
					return fmt.Errorf("expected certificate %q, got %q", "Y2VydGlmaWNhdGUtZGF0YQ==", cert.Certificate)

				}
				return nil
			},
			false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			rr, err := New(tc.in)
			if err != nil && !tc.err {
				t.Fatal(err)
			}
			if err = tc.fn(rr); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func FuzzNew(f *testing.F) {
	f.Add(`. 5 IN SVCB 50 five.test. alpn=part1\,\p\a\r\t2\044part3\092,part4\092\\`)
	f.Add(`miek.nl. IN 3600 MX 15 mx.miek.nl.`)
	f.Add(`\"\" IN 3600 (MX) (15) (mx.miek.nl.)`)
	start := time.Now()
	f.Fuzz(func(t *testing.T, s string) {
		New(s)
		dnsfuzz.Stop(t, start)
	})
}
