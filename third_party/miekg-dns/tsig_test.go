package dns

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"log"
	"net"
	"testing"
)

var tsigSecret = []byte("blaat")

// ExampleTSIG_notify shows how to create a notify message with a TSIG signature.
func ExampleTSIG_notify() {
	m := NewMsg("example.org.", TypeSOA)
	m.Authoritative = true
	m.Opcode = OpcodeNotify
	m.Pseudo = []RR{NewTSIG("key.example.com.", HmacSHA256, 0)}

	option := &TSIGOption{}
	hmac := HmacTSIG{Secret: tsigSecret}
	if err := TSIGSign(m, hmac, option); err != nil {
		log.Fatalf("Failed to sign: %s", err)
	}

	c := new(Client)
	// Use this if the notify must come from a particular source address.
	c.Transport = NewTransport()
	c.Dialer.LocalAddr = &net.UDPAddr{IP: net.ParseIP("198.51.100.1")}

	r, _, err := c.Exchange(context.TODO(), m, "udp", "8.8.4.4:53")
	if err != nil {
		log.Fatalf("Failed to sent notify: %s", err)
	}
	if r.Rcode == RcodeSuccess {
		log.Fatalf("Failed to sent notify: %s", err)
	}
}

func msgfn() *Msg {
	m := NewMsg("miek.nl.", TypeMX)
	m.ID = 3
	m.Pseudo = []RR{NewTSIG("example.", HmacSHA256, 0)}
	return m
}

func TestTSIG(t *testing.T) {
	testcases := []struct {
		name   string
		preFn  func(m *Msg) // transform before signing
		postFn func(m *Msg) // transform before verifying
		err    error
	}{
		{"signverify", nil, nil, nil},
		{"signverify-edns", func(m *Msg) { m.Security = true }, nil, nil},
		{"signverify-changed-id", nil, func(m *Msg) { binary.BigEndian.PutUint16(m.Data[0:2], 42) }, nil},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			m := msgfn()
			if tc.preFn != nil {
				tc.preFn(m)
			}

			option := TSIGOption{}
			hmac := HmacTSIG{Secret: tsigSecret}
			if err := TSIGSign(m, hmac, &option); err != nil {
				t.Fatalf("failed to sign: %s", err)
			}
			if tc.postFn != nil {
				tc.postFn(m)
			}

			option.RequestMAC = "" // Negate this from TSIGSign, as TSIGVerify is supposed to be running on a different machine normally.

			err := TSIGVerify(m, hmac, &option)
			if err != tc.err {
				t.Fatalf("expected %v error, got: %s", tc.err, err)
			}
		})
	}
}

func TestTSIG_Interop(t *testing.T) {
	const (
		keyname    = "tsig-key.example.org."
		timeSigned = int64(1700000000)
		secret     = "c2VjcmV0IHBhc3N3b3Jk"
	)

	// These testcases are computed from dnspython and used here to test our impl. adherence to standards.
	// See #1165 (https://codeberg.org/miekg/dns/pulls/1165/files).
	testcases := []struct {
		name, algorithm, prev string
		timersOnly            bool
		wire                  string // hex, the signed message with the TSIG RR included
		start                 int    // where the TSIG RR starts in wire
		mac                   string // hex, the MAC TSIGSign must reproduce
	}{
		{
			name:      "sha256-first",
			algorithm: HmacSHA256,
			wire:      "123481000001000200000001076578616d706c65036f72670000fc00010178c00c0001000100000e1000040a000001c00c0010000100000e10000c0b763d73706631202d616c6c08747369672d6b6579076578616d706c65036f72670000fa00ff00000000003d0b686d61632d7368613235360000006553f100012c002040b62032a9d7ad98e9c6c16ae5dd48513e8d247123277a75898559b1545e96bf123400000000",
			start:     71,
			mac:       "40b62032a9d7ad98e9c6c16ae5dd48513e8d247123277a75898559b1545e96bf",
		},
		{
			name:      "sha256-chained",
			algorithm: HmacSHA256,
			prev:      "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1",
			wire:      "123481000001000200000001076578616d706c65036f72670000fc00010178c00c0001000100000e1000040a000001c00c0010000100000e10000c0b763d73706631202d616c6c08747369672d6b6579076578616d706c65036f72670000fa00ff00000000003d0b686d61632d7368613235360000006553f100012c0020c5428a20088553636330720c093703e3170e3793f55fb9809f3e7a8df110a0b8123400000000",
			start:     71,
			mac:       "c5428a20088553636330720c093703e3170e3793f55fb9809f3e7a8df110a0b8",
		},
		{
			name:      "sha512-first",
			algorithm: HmacSHA512,
			wire:      "123481000001000200000001076578616d706c65036f72670000fc00010178c00c0001000100000e1000040a000001c00c0010000100000e10000c0b763d73706631202d616c6c08747369672d6b6579076578616d706c65036f72670000fa00ff00000000005d0b686d61632d7368613531320000006553f100012c004039b2b9ce6259efe228c3dfbfc19ac12d9bb602e744b3a17ae6e1700c4148afbfcac36bd6105e92fc28df76fb8bfd23dc56c80d58b824d142fdb27cce5b7d16a3123400000000",
			start:     71,
			mac:       "39b2b9ce6259efe228c3dfbfc19ac12d9bb602e744b3a17ae6e1700c4148afbfcac36bd6105e92fc28df76fb8bfd23dc56c80d58b824d142fdb27cce5b7d16a3",
		},
		{
			name:      "sha512-chained",
			algorithm: HmacSHA512,
			prev:      "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1",
			wire:      "123481000001000200000001076578616d706c65036f72670000fc00010178c00c0001000100000e1000040a000001c00c0010000100000e10000c0b763d73706631202d616c6c08747369672d6b6579076578616d706c65036f72670000fa00ff00000000005d0b686d61632d7368613531320000006553f100012c0040ddd7bfbb12d525de3157a0cf47e942cd8e0fd1184ea85bba5de597cd714f80ec2ded7e691aa7c1ef8789a3e4936c23faf3270a24d9dbe898268e6554e5d51f79123400000000",
			start:     71,
			mac:       "ddd7bfbb12d525de3157a0cf47e942cd8e0fd1184ea85bba5de597cd714f80ec2ded7e691aa7c1ef8789a3e4936c23faf3270a24d9dbe898268e6554e5d51f79",
		},
		{
			name:       "sha256-timersonly",
			algorithm:  HmacSHA256,
			prev:       "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1",
			timersOnly: true,
			wire:       "123481000001000200000001076578616d706c65036f72670000fc00010178c00c0001000100000e1000040a000001c00c0010000100000e10000c0b763d73706631202d616c6c08747369672d6b6579076578616d706c65036f72670000fa00ff00000000003d0b686d61632d7368613235360000006553f100012c0020313a0889007b9677173bdfaece96d24136f2b9a11e5475fb462bfc29c56f352b123400000000",
			start:      71,
			mac:        "313a0889007b9677173bdfaece96d24136f2b9a11e5475fb462bfc29c56f352b",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := hex.DecodeString(tc.wire)
			if err != nil {
				t.Fatal(err)
			}
			// the bytes as the signer hashed them: the wire without the TSIG RR,
			// the additional count without the TSIG RR too
			unsigned := make([]byte, 0, tc.start)
			unsigned = append(unsigned, wire[:10]...)
			unsigned = binary.BigEndian.AppendUint16(unsigned, binary.BigEndian.Uint16(wire[10:12])-1)
			unsigned = append(unsigned, wire[12:tc.start]...)

			m := &Msg{Data: unsigned}
			m.Data = unsigned
			m.Unpack()
			m.Data = nil // unsigned was only need to do the unpack
			m.Pseudo = []RR{NewTSIG(keyname, tc.algorithm, 300, timeSigned)}

			key, _ := base64.StdEncoding.DecodeString(secret)
			if err = TSIGSign(m, HmacTSIG{Secret: key}, &TSIGOption{RequestMAC: tc.prev, TimersOnly: tc.timersOnly}); err != nil {
				t.Fatal(err)
			}
			if got := m.Pseudo[0].(*TSIG).MAC; got != tc.mac {
				t.Fatalf("expected %s, got %s", tc.mac, got)
			}
		})
	}
}
