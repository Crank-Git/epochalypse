package dns_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net/netip"
	"os"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnstest"
	"codeberg.org/miekg/dns/internal/bin"
	"codeberg.org/miekg/dns/internal/dnsfuzz"
)

func ExampleMsg() {
	m := dns.NewMsg("miek.nl.", dns.TypeMX)
	c := new(dns.Client)
	r, _, err := c.Exchange(context.TODO(), m, "udp", "127.0.0.1:53")
	if err != nil {
		log.Fatal(err)
	}
	if m, ok := r.Answer[0].(*dns.MX); ok {
		fmt.Println(m.Mx)
	}
	if n, ok := r.Pseudo[0].(*dns.NSID); ok {
		fmt.Println(n.Nsid)
	}
	for rr := range r.All() {
		fmt.Println(rr)
	}
}

func ExampleMsg_dNSSEC() {
	m := dns.NewMsg("miek.nl.", dns.TypeMX)
	m.UDPSize = dns.DefaultMsgSize
	m.Security = true
	dns.Exchange(context.TODO(), m, "udp", "127.0.0.1:53")
	// handle returned message.
}

func TestMsgBinary(t *testing.T) {
	testcases := []struct {
		name string
		buf  []byte
		fn   func(*dns.Msg, error) error
	}{
		{
			// m := dns.NewMsg("example.org.", dns.TypeMX)
			// m.Answer = []dns.RR{dnstest.New("example.org. IN SOA linode.atoom.net. miek\\.miek.nl. 1 3600 3600 3600 3600")}
			"soa-mbox",
			[]byte{148, 44, 1, 0, 0, 1, 0, 1, 0, 0, 0, 0, 7, 101, 120, 97, 109, 112, 108, 101, 3, 111, 114, 103, 0, 0, 15, 0, 1, 192, 12, 0, 6, 0, 1, 0, 0, 14, 16, 0, 52, 6, 108, 105, 110, 111, 100, 101, 5, 97, 116, 111, 111, 109, 3, 110, 101, 116, 0, 9, 109, 105, 101, 107, 46, 109, 105, 101, 107, 2, 110, 108, 0, 0, 0, 0, 1, 0, 0, 14, 16, 0, 0, 14, 16, 0, 0, 14, 16, 0, 0, 14, 16},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr != nil {
					return unpackerr
				}
				if len(m.Answer) != 1 {
					return errors.New("expected answer section")
				}
				s, ok := m.Answer[0].(*dns.SOA)
				if !ok {
					return errors.New("expected SOA")
				}
				if s.Mbox != `miek\.miek.nl.` {
					return errors.New("SOA Mbox is not correct")
				}
				return nil
			},
		},

		{
			"edns0-subnet",
			[]byte{149, 112, 0, 16, 0, 1, 0, 0, 0, 0, 0, 1, 1, 97, 4, 109, 105, 69, 75, 2, 78, 76, 0, 0, 1, 0, 1, 0, 0, 41, 5, 120, 0, 0, 128, 0, 0, 11, 0, 8, 0, 7, 0, 1, 24, 0, 14, 128, 63},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr != nil {
					return unpackerr
				}
				if len(m.Pseudo) == 0 {
					return errors.New("expected pseudo section")
				}
				s, ok := m.Pseudo[0].(*dns.SUBNET)
				if !ok {
					return errors.New("expected EDNS0 SUBNET")
				}
				const addr = "14.128.63.0"
				if s.Address != netip.MustParseAddr(addr) {
					return errors.New("expected address: " + addr)
				}
				return nil
			},
		},
		{
			"edns0-subnet",
			[]byte{255, 234, 0, 16, 0, 1, 0, 0, 0, 0, 0, 1, 7, 99, 111, 114, 101, 68, 110, 83, 2, 105, 111, 0, 0, 28, 0, 1, 0, 0, 41, 5, 120, 0, 0, 128, 0, 0, 11, 0, 8, 0, 7, 0, 1, 24, 0, 62, 212, 234},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr != nil {
					return unpackerr
				}
				if len(m.Pseudo) == 0 {
					return errors.New("expected pseudo section")
				}
				s, ok := m.Pseudo[0].(*dns.SUBNET)
				if !ok {
					return errors.New("expected EDNS0 SUBNET")
				}
				const addr = "62.212.234.0"
				if s.Address != netip.MustParseAddr(addr) {
					return errors.New("expected address: " + addr)
				}
				return nil
			},
		},
		{
			"opt-and-tsig",
			[]byte{148, 7, 1, 32, 0, 1, 0, 0, 0, 0, 0, 2, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0, 6, 0, 1, 0, 0, 41, 4, 208, 0, 0, 0, 0, 0, 12, 0, 10, 0, 8, 73, 65, 52, 201, 253, 43, 171, 193, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0, 250, 0, 255, 0, 0, 0, 0, 0, 93, 11, 104, 109, 97, 99, 45, 115, 104, 97, 53, 49, 50, 0, 0, 0, 105, 104, 143, 225, 1, 44, 0, 64, 195, 169, 191, 31, 144, 147, 160, 197, 245, 76, 217, 137, 234, 208, 246, 112, 113, 12, 208, 172, 99, 181, 29, 108, 140, 62, 197, 130, 116, 207, 127, 178, 163, 16, 242, 203, 41, 135, 60, 218, 187, 237, 181, 106, 91, 34, 125, 38, 190, 56, 117, 43, 76, 212, 161, 165, 61, 214, 193, 180, 117, 1, 27, 129, 148, 7, 0, 0, 0, 0},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr != nil {
					return unpackerr
				}
				if len(m.Pseudo) == 0 {
					return errors.New("expected pseudo section")
				}
				_, ok := m.Pseudo[1].(*dns.TSIG) // 0 is COOKIE
				if !ok {
					return errors.New("expected TSIG")
				}
				return nil
			},
		},
		{
			"opt-and-extra-tsig",
			[]byte{148, 7, 1, 32, 0, 1, 0, 0, 0, 0, 0, 3, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0, 6, 0, 1, 0, 0, 41, 4, 208, 0, 0, 0, 0, 0, 12, 0, 10, 0, 8, 73, 65, 52, 201, 253, 43, 171, 193, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0, 250, 0, 255, 0, 0, 0, 0, 0, 93, 11, 104, 109, 97, 99, 45, 115, 104, 97, 53, 49, 50, 0, 0, 0, 105, 104, 143, 225, 1, 44, 0, 64, 195, 169, 191, 31, 144, 147, 160, 197, 245, 76, 217, 137, 234, 208, 246, 112, 113, 12, 208, 172, 99, 181, 29, 108, 140, 62, 197, 130, 116, 207, 127, 178, 163, 16, 242, 203, 41, 135, 60, 218, 187, 237, 181, 106, 91, 34, 125, 38, 190, 56, 117, 43, 76, 212, 161, 165, 61, 214, 193, 180, 117, 1, 27, 129, 148, 7, 0, 0, 0, 0, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0, 250, 0, 255, 0, 0, 0, 0, 0, 93, 11, 104, 109, 97, 99, 45, 115, 104, 97, 53, 49, 50, 0, 0, 0, 105, 104, 143, 225, 1, 44, 0, 64, 195, 169, 191, 31, 144, 147, 160, 197, 245, 76, 217, 137, 234, 208, 246, 112, 113, 12, 208, 172, 99, 181, 29, 108, 140, 62, 197, 130, 116, 207, 127, 178, 163, 16, 242, 203, 41, 135, 60, 218, 187, 237, 181, 106, 91, 34, 125, 38, 190, 56, 117, 43, 76, 212, 161, 165, 61, 214, 193, 180, 117, 1, 27, 129, 148, 7, 0, 0, 0, 0},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr == nil {
					return errors.New("expected Unpack failure 'multiple TSIG RRs'")
				}
				return nil
			},
		},
		{
			"opt-and-tsig-extra-should-empty",
			[]byte{148, 7, 1, 32, 0, 1, 0, 0, 0, 0, 0, 2, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0, 6, 0, 1, 0, 0, 41, 4, 208, 0, 0, 0, 0, 0, 12, 0, 10, 0, 8, 73, 65, 52, 201, 253, 43, 171, 193, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0, 250, 0, 255, 0, 0, 0, 0, 0, 93, 11, 104, 109, 97, 99, 45, 115, 104, 97, 53, 49, 50, 0, 0, 0, 105, 104, 143, 225, 1, 44, 0, 64, 195, 169, 191, 31, 144, 147, 160, 197, 245, 76, 217, 137, 234, 208, 246, 112, 113, 12, 208, 172, 99, 181, 29, 108, 140, 62, 197, 130, 116, 207, 127, 178, 163, 16, 242, 203, 41, 135, 60, 218, 187, 237, 181, 106, 91, 34, 125, 38, 190, 56, 117, 43, 76, 212, 161, 165, 61, 214, 193, 180, 117, 1, 27, 129, 148, 7, 0, 0, 0, 0},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr != nil {
					return unpackerr
				}
				if len(m.Extra) != 0 {
					return errors.New("expected additional section to be empty")
				}
				return nil
			},
		},
		{
			"unknown-edns0-code20",
			//  edns255 := &dns.ERFC3597{EDNS0Code: 255, Code: hex.EncodeToString([]byte("hallo"))}
			//  m.Pseudo = append(m.Pseudo, edns255)
			[]byte{0, 3, 1, 0, 0, 1, 0, 0, 0, 0, 0, 1, 3, 119, 119, 119, 7, 101, 120, 97, 109, 112, 108, 101, 3, 111, 114, 103, 0, 0, 1, 0, 1, 0, 0, 41, 0, 0, 0, 0, 0, 0, 0, 9, 0, 255, 0, 5, 104, 97, 108, 108, 111},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr != nil {
					return unpackerr
				}
				if len(m.Pseudo) != 1 {
					return errors.New("expected pseudo section to carry an option")
				}
				x := m.Pseudo[0].(*dns.ERFC3597)
				if x.EDNS0Code != 255 {
					return fmt.Errorf("expected code 255, got %d", x.EDNS0Code)
				}
				return nil
			},
		},
		{
			"zoneversion-empty",
			//  m.UDPSize = 1232
			//  m.Pseudo = []dns.RR{&dns.ZONEVERSION{}}
			[]byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 1, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 4, 't', 'e', 's', 't', 0, 0, 1, 0, 1, 0, 0, 41, 0x04, 0xd0, 0, 0, 0, 0, 0, 4, 0, 19, 0, 0},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr != nil {
					return unpackerr
				}
				if m.UDPSize != 1232 || len(m.Pseudo) != 1 {
					return errors.New("expected UDPSize 1232 and a ZONEVERSION option")
				}
				if _, ok := m.Pseudo[0].(*dns.ZONEVERSION); !ok {
					return fmt.Errorf("expected ZONEVERSION, got %T", m.Pseudo[0])
				}
				return nil
			},
		},
		{
			"fuzzing-find-#932",
			[]byte{48, 48, 48, 48, 0, 2, 0, 2, 0, 0, 0, 0, 0, 48, 48, 48, 48, 0, 48, 48, 48, 48, 0, 0, 9, 48, 48, 48, 48, 48, 48, 0, 0, 192, 32, 48, 48, 48, 48, 48, 48, 48, 48, 0, 16, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr != nil {
					return unpackerr
				}
				// the message is a bit fubar, but we did crash on it, when packing again.
				m.Pack()
				return nil
			},
		},
		{
			"question without class", // dns.NewMsg("example.", dns.TypeAAAA) -> println(bin.Bytes(m.Data)) and stripped away the class; last 2 bytes.
			[]byte{175, 231, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 7, 101, 120, 97, 109, 112, 108, 101, 0, 0, 28},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr == nil {
					return fmt.Errorf("unpacking without class should error")
				}
				return nil
			},
		},
		{
			"unpack error type", // same as above, but check the error with errors.As.
			[]byte{175, 231, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 7, 101, 120, 97, 109, 112, 108, 101, 0, 0, 28},
			func(m *dns.Msg, unpackerr error) error {
				if _, ok := errors.AsType[*dns.UnpackError](fmt.Errorf("wrapped: %w", unpackerr)); !ok {
					return fmt.Errorf("expected *dns.UnpackError, got %T", unpackerr)
				}
				return nil
			},
		},
		/*
			;; OPT PSEUDOSECTION:
			; EDNS: version 0; flags:; udp: 1232
			;; QUESTION SECTION:
			;example.       IN       A
			;; ADDITIONAL SECTION:
			example.        0       IN      TXT     "preserve-me"
			;; TSIG PSEUDOSECTION:
			; key.example.  0       CLASS255        TSIG     hmac-sha256. 20260920150926 300 32 FEEB41C4A3597B9CC34F26CF8B249B34B7F617923DD12C67BFCD05DC3DCC7946 4660 0 0
		*/
		{
			"Extra opt+txt+sig",
			[]byte{18, 52, 1, 0, 0, 1, 0, 0, 0, 0, 0, 3, 7, 101, 120, 97, 109, 112, 108, 101, 0, 0, 1, 0, 1, 0, 0, 41, 4, 208, 0, 0, 0, 0, 0, 0, 7, 101, 120, 97, 109, 112, 108, 101, 0, 0, 16, 0, 1, 0, 0, 0, 0, 0, 12, 11, 112, 114, 101, 115, 101, 114, 118, 101, 45, 109, 101, 3, 107, 101, 121, 7, 101, 120, 97, 109, 112, 108, 101, 0, 0, 250, 0, 255, 0, 0, 0, 0, 0, 61, 11, 104, 109, 97, 99, 45, 115, 104, 97, 50, 53, 54, 0, 0, 0, 106, 175, 247, 38, 1, 44, 0, 32, 254, 235, 65, 196, 163, 89, 123, 156, 195, 79, 38, 207, 139, 36, 155, 52, 183, 246, 23, 146, 61, 209, 44, 103, 191, 205, 5, 220, 61, 204, 121, 70, 18, 52, 0, 0, 0, 0},
			func(m *dns.Msg, unpackerr error) error {
				if m.UDPSize != 1232 {
					t.Fatalf("expected UDPSize to be 1232, got %d", m.UDPSize)
				}
				if len(m.Pseudo) != 1 {
					t.Fatalf("expected Pseudo count to be 1, got %d", len(m.Pseudo))
				}
				if _, ok := m.Pseudo[0].(*dns.TSIG); !ok {
					t.Fatalf("expected TSIG, got %T", m.Pseudo[0])
				}
				if len(m.Extra) != 1 {
					t.Fatalf("expected Extra count to be 1, got %d", len(m.Extra))
				}
				txt, ok := m.Extra[0].(*dns.TXT)
				if !ok || len(txt.Txt) != 1 || txt.Txt[0] != "preserve-me" {
					t.Fatal("failed to find TXT record")
				}
				return nil
			},
		},
		{
			"Label longer than the octets left",
			[]byte{0, 1, 0x80, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 2, 0, 1, 0, 0, 0, 60, 0, 5, 2, 'a', 'b', 3, 0},
			func(m *dns.Msg, unpackerr error) error {
				if unpackerr == nil {
					return fmt.Errorf("unpacking with label overflow should error")
				}
				return nil
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			m := &dns.Msg{Data: tc.buf}
			err := m.Unpack()
			if err := tc.fn(m, err); err != nil {
				t.Logf("%s\n", bin.Dump(m.Data))
				t.Fatal(err)
			}
		})
	}
}

func TestMsgPackError(t *testing.T) {
	m := dns.NewMsg("example.", dns.TypeAAAA)
	m.Question[0].Header().Name = "example" // not fully qualified
	err := m.Pack()
	var pe *dns.PackError
	if !errors.As(fmt.Errorf("wrapped: %w", err), &pe) {
		t.Fatalf("expected *dns.PackError, got %T", err)
	}
}

func TestMsg(t *testing.T) {
	const msgArcount = 10 // offset in the message where the Arcount is, 2 octets long.
	a := new(dns.RFC3597)
	if err := a.ToRFC3597(dnstest.New("example. IN A 192.0.2.1")); err != nil {
		t.Fatal(err)
	}
	null := new(dns.RFC3597)
	if err := null.ToRFC3597(&dns.NULL{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}}); err != nil {
		t.Fatal(err)
	}
	testcases := []struct {
		name   string
		makeFn func() *dns.Msg
		testFn func(m *dns.Msg) error
	}{
		{
			"extendedrcode",
			func() *dns.Msg {
				m := dns.NewMsg("example.org.", dns.TypeMX)
				m.ID = 3
				m.Rcode = dns.RcodeBadTime
				return m
			},
			func(r *dns.Msg) error {
				if r.Rcode != dns.RcodeBadTime {
					return fmt.Errorf("expected %s, got %s", dns.RcodeToString[dns.RcodeBadTime], dns.RcodeToString[r.Rcode])
				}
				return nil
			},
		},
		{
			"security",
			func() *dns.Msg { m := dns.NewMsg("example.org.", dns.TypeMX); m.ID = 3; m.Security = true; return m },
			func(r *dns.Msg) error {
				if !r.Security {
					return fmt.Errorf("expected %t, got %t", r.Security, !r.Security)
				}
				arcount := binary.BigEndian.Uint16(r.Data[msgArcount:])
				if arcount != 1 {
					return fmt.Errorf("expected arcount to be 1, got %d", arcount)
				}
				return nil
			},
		},
		{
			"ednsz",
			func() *dns.Msg {
				m := dns.NewMsg("example.org.", dns.TypeMX)
				m.ID = 3
				m.Z = 0x0080
				return m
			},
			func(r *dns.Msg) error {
				if r.Z != 0x0080 {
					return fmt.Errorf("expected %#04x, got %#04x", 0x0080, r.Z)
				}
				arcount := binary.BigEndian.Uint16(r.Data[msgArcount:])
				if arcount != 1 {
					return fmt.Errorf("expected arcount to be 1, got %d", arcount)
				}
				return nil
			},
		},
		{
			"ednsz-security",
			func() *dns.Msg {
				m := dns.NewMsg("example.org.", dns.TypeMX)
				m.ID = 3
				m.Security, m.CompactAnswers, m.Delegation = true, true, true
				return m
			},
			func(r *dns.Msg) error {
				if r.Z != 0 {
					return fmt.Errorf("expected %#04x, got %#04x", 0, r.Z)
				}
				return nil
			},
		},
		{
			"security+nsd",
			func() *dns.Msg {
				m := dns.NewMsg("example.org.", dns.TypeMX)
				m.ID = 3
				m.Security = true
				m.Pseudo = []dns.RR{&dns.NSID{}}
				return m
			},
			func(r *dns.Msg) error {
				if !r.Security {
					return fmt.Errorf("expected %t, got %t", r.Security, !r.Security)
				}
				arcount := binary.BigEndian.Uint16(r.Data[msgArcount:])
				if arcount != 1 { // nsid and DO bit are stored in a single record
					return fmt.Errorf("expected arcount to be 1, got %d", arcount)
				}
				if x := len(r.Pseudo); x != 1 {
					return fmt.Errorf("expected len(pseudo) to be 1, got %d", x)
				}
				return nil
			},
		},
		{
			"security+tsig-nosign+nsid",
			func() *dns.Msg {
				m := dns.NewMsg("example.org.", dns.TypeMX)
				m.ID = 3
				m.Security = true
				m.Pseudo = []dns.RR{&dns.NSID{}}
				m.Pseudo = append(m.Pseudo, dns.NewTSIG("example.", dns.HmacSHA256, 0))
				return m
			},
			func(r *dns.Msg) error {
				if !r.Security {
					return fmt.Errorf("expected %t, got %t", !r.Security, r.Security)
				}
				arcount := binary.BigEndian.Uint16(r.Data[10:])
				if arcount != 2 { // nsid and DO bit are stored in a single record + tsig
					return fmt.Errorf("expected arcount to be 2, got %d", arcount)
				}
				if x := len(r.Pseudo); x != 2 {
					return fmt.Errorf("expected len(pseudo) to be 2, got %d", x)
				}
				return nil
			},
		},
		{
			"edns0+tsig",
			func() *dns.Msg {
				m := dns.NewMsg("example.", dns.TypeA)
				m.UDPSize = dns.DefaultMsgSize
				m.Pseudo = append(m.Pseudo,
					&dns.COOKIE{Cookie: "0102030405060708"},
					dns.NewTSIG("example.", dns.HmacSHA256, 0),
				)
				if err := dns.TSIGSign(m, dns.HmacTSIG{Secret: []byte("Wg==")}, &dns.TSIGOption{}); err != nil {
					panic(err)
				}
				return m
			},
			func(r *dns.Msg) error {
				if len(r.Pseudo) != 2 {
					return fmt.Errorf("expected two pseudo records, got %d", len(r.Pseudo))
				}
				if _, ok := r.Pseudo[0].(*dns.COOKIE); !ok {
					return fmt.Errorf("expected COOKIE first, got %T", r.Pseudo[0])
				}
				if _, ok := r.Pseudo[1].(*dns.TSIG); !ok {
					return fmt.Errorf("expected TSIG last, got %T", r.Pseudo[1])
				}
				if err := dns.TSIGVerify(r, dns.HmacTSIG{Secret: []byte("Wg==")}, &dns.TSIGOption{}); err != nil {
					return fmt.Errorf("failed to verify TSIG: %w", err)
				}
				if err := r.Pack(); err != nil {
					return fmt.Errorf("failed to repack message: %w", err)
				}
				return nil
			},
		},
		{
			"tsig",
			func() *dns.Msg {
				m := dns.NewMsg("example.org.", dns.TypeMX)
				m.ID = 3
				m.Pseudo = []dns.RR{dns.NewTSIG("example.", dns.HmacSHA256, 0)}
				return m
			},
			func(r *dns.Msg) error {
				if r.Security {
					return fmt.Errorf("expected %t, got %t", !r.Security, r.Security)
				}
				arcount := binary.BigEndian.Uint16(r.Data[10:])
				if arcount != 1 {
					return fmt.Errorf("expected arcount to be q, got %d", arcount)
				}
				if x := len(r.Pseudo); x != 1 {
					return fmt.Errorf("expected len(pseudo) to be 1, got %d", x)
				}
				if x := len(r.Extra); x != 0 {
					return fmt.Errorf("expected len(extra) to be 0, got %d", x)
				}
				return nil
			},
		},
		{
			"nsec3",
			func() *dns.Msg {
				m := dns.NewMsg("miek.nl.", dns.TypeMX)
				m.ID = 3
				nsec3 := dnstest.New("k36vo59bkum4osckkrd8tvibdgr0njbc.nl. 599 IN NSEC3 1 0 0 - K36VONMLM2T8IF3G8P5AV864OHLTB7K7 NS SOA TXT RRSIG DNSKEY NSEC3PARAM")
				m.Answer = []dns.RR{nsec3}
				return m
			},
			func(r *dns.Msg) error {
				expect := []byte{0, 3, 1, 0, 0, 1, 0, 1, 0, 0, 0, 0, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0, 15, 0, 1, 32, 107, 51, 54, 118, 111, 53, 57, 98, 107, 117, 109, 52, 111, 115, 99, 107, 107, 114, 100, 56, 116, 118, 105, 98, 100, 103, 114, 48, 110, 106, 98, 99, 192, 17, 0, 50, 0, 1, 0, 0, 2, 87, 0, 35, 1, 0, 0, 0, 0, 20, 160, 205, 252, 94, 213, 176, 186, 137, 60, 112, 70, 74, 175, 160, 196, 196, 107, 213, 158, 135, 0, 7, 34, 0, 128, 0, 0, 2, 144}
				if !bytes.Equal(r.Data, expect) {
					return fmt.Errorf("Msg octets do not match")
				}
				return nil

			},
		},
		{
			"pseudo-nsid",
			func() *dns.Msg {
				m := dns.NewMsg("miek.nl.", dns.TypeMX)
				m.ID = 3
				m.Pseudo = []dns.RR{&dns.NSID{}}
				return m
			},
			func(r *dns.Msg) error {
				// 41 is OPT after the zeros, 04 -> rdlength, 03 -> code of NSID, 00 -> "rdlength" of NSID
				expect := []byte{0, 3, 1, 0, 0, 1, 0, 0, 0, 0, 0, 1, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0, 15, 0, 1, 0, 0, 41, 0, 0, 0, 0, 0, 0, 0, 4, 0, 3, 0, 0}
				if !bytes.Equal(r.Data, expect) {
					return fmt.Errorf("Msg octets do not match")
				}
				return nil
			},
		},
		{
			"question-mx",
			func() *dns.Msg {
				m := dns.NewMsg("miek.nl.", dns.TypeMX)
				m.ID = 3
				return m
			},
			func(r *dns.Msg) error {
				expect := []byte{0, 3, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0, 15, 0, 1}
				if !bytes.Equal(r.Data, expect) {
					return fmt.Errorf("Msg octets do not match")
				}
				return nil
			},
		},
		{
			"ede",
			func() *dns.Msg {
				m := dns.NewMsg("miek.nl.", dns.TypeMX)
				m.ID = 3
				m.Pseudo = []dns.RR{&dns.EDE{
					InfoCode:  dns.ExtendedErrorDNSSECIndeterminate,
					ExtraText: "Some extra text",
				}}
				return m
			},
			func(r *dns.Msg) error {
				expect := []byte{
					0, 3, 1, 0, 0, 1, 0, 0, 0, 0, 0, 1, 4, 109, 105, 101, 107, 2, 110, 108, 0, 0,
					15, 0, 1, 0, 0, 41, 0, 0, 0, 0, 0, 0, 0, 21, 0, 15, 0, 17, 0, 5, 83, 111, 109,
					101, 32, 101, 120, 116, 114, 97, 32, 116, 101, 120, 116,
				}
				if !bytes.Equal(r.Data, expect) {
					return fmt.Errorf("Msg octets do not match")
				}
				return nil
			},
		},
		{
			"rfc3597",
			func() *dns.Msg {
				m := dns.NewMsg("example.", dns.TypeA)
				m.Answer = []dns.RR{
					dnstest.New("example. IN TYPE65534 \\# 5 084ff00001"),
					dnstest.New("example. IN TYPE65280 \\# 0"),
					a,
					null,
				}
				return m
			},
			func(r *dns.Msg) error {
				want := []uint16{65534, 65280, dns.TypeA, dns.TypeNULL}
				if len(r.Answer) != len(want) {
					return fmt.Errorf("expected %d answers, got %d", len(want), len(r.Answer))
				}
				for i, rr := range r.Answer {
					if got := dns.RRToType(rr); got != want[i] {
						return fmt.Errorf("answer %d: expected TYPE%d, got TYPE%d", i, want[i], got)
					}
				}
				return r.Pack()
			},
		},
		{
			"malformed-a",
			func() *dns.Msg {
				// name=. type=A class=IN ttl=0 rdlength={0, 0} -> and no rdata
				return &dns.Msg{Data: []byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0}}
			},
			func(r *dns.Msg) error {
				return nil
			},
		},
		{
			"malformed-aaaa",
			func() *dns.Msg {
				// name=. type=AAAA class=IN ttl=0 rdlength={0,1} -> and then 2 as malformed address
				return &dns.Msg{Data: []byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 28, 0, 0, 0, 0, 0, 1, 2}}
			},
			func(r *dns.Msg) error {
				return nil
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.makeFn()
			m.Pack()

			r := new(dns.Msg)
			r.Data = m.Data
			r.Unpack()
			if err := tc.testFn(r); err != nil {
				t.Fatalf("failed testFn: %s", err)
			}
		})
	}

}

func FuzzMsgPack(f *testing.F) {
	binaries := []string{"dig-mx-miek.nl", "dig+do+nsid-a-miek.nl"}
	for _, binary := range binaries {
		buf, _ := os.ReadFile("testdata/" + binary)
		f.Add(buf)
	}
	start := time.Now()
	f.Fuzz(func(t *testing.T, b []byte) {
		m := &dns.Msg{Data: b}
		m.Unpack()
		dnsfuzz.Stop(t, start)
	})
}

func TestMsgReadAll(t *testing.T) {
	m := dns.NewMsg("example.org.", dns.TypeA)
	m.Pack()

	done := make(chan struct{})
	go func() {
		io.ReadAll(m) // should not hang
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected io.ReadAll to complete, but hung")
	}
}

func TestMsgFollow(t *testing.T) {
	// answerK returns at most size octets: a chain of k questions, then A records owned by its last name.
	answerK := func(k, size int) []byte {
		b := []byte{0, 1, 0x84, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		head := len(b)
		b = append(b, 1, 'a', 0, 0, 1, 0, 1)
		for range k - 1 {
			prev := head
			head = len(b)
			b = append(b, 1, 'a', 0xC0|byte(prev>>8), byte(prev), 0, 1, 0, 1)
		}
		an := 0
		for len(b)+16 <= size {
			b = append(b, 0xC0|byte(head>>8), byte(head), 0, 1, 0, 1, 0, 0, 0x0e, 0x10, 0, 4, 192, 0, 2, byte(an))
			an++
		}
		binary.BigEndian.PutUint16(b[4:], uint16(k))
		binary.BigEndian.PutUint16(b[6:], uint16(an))
		return b
	}

	t.Run("k=1", func(t *testing.T) {
		m := &dns.Msg{Data: answerK(1, dns.MaxMsgSize)}
		if err := m.Unpack(); err != nil {
			t.Fatal(err)
		}
		if len(m.Answer) != 4094 {
			t.Fatalf("expected 4094 answers, got %d", len(m.Answer))
		}
	})

	t.Run("k=126", func(t *testing.T) {
		m := &dns.Msg{Data: answerK(126, dns.MaxMsgSize)}
		if err := m.Unpack(); err == nil || err.Error() != "dns unpack: too many compression pointers in message" {
			t.Fatalf("expected the message budget to fail, got %v", err)
		}
	})
}
