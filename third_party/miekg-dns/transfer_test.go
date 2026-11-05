package dns_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnstest"
)

var testTransferData = []dns.RR{
	dnstest.New("miek.nl. IN SOA linode.atoom.net. miek.miek.nl. 2009032800 21600 7200 604800 3600"),
	dnstest.New("x.miek.nl. IN A 10.0.0.1"),
	dnstest.New("miek.nl. IN MX 1 x.miek.nl."),
	dnstest.New("miek.nl. IN SOA linode.atoom.net. miek.miek.nl. 2009032800 21600 7200 604800 3600"),
}

var testTransferDataIncrementalData = []dns.RR{
	dnstest.New("miek.nl. IN SOA linode.atoom.net. miek.miek.nl. 2009032802 21600 7200 604800 3600"),
	dnstest.New("miek.nl. IN SOA linode.atoom.net. miek.miek.nl. 2009032800 21600 7200 604800 3600"),
	dnstest.New("x.miek.nl. IN A 10.0.0.1"),
	dnstest.New("miek.nl. IN MX 1 x.miek.nl."),
	dnstest.New("miek.nl. IN SOA linode.atoom.net. miek.miek.nl. 2009032802 21600 7200 604800 3600"),
	dnstest.New("x.miek.nl. IN A 10.0.0.5"),
	dnstest.New("miek.nl. IN MX 10 x.miek.nl."),
	dnstest.New("miek.nl. IN SOA linode.atoom.net. miek.miek.nl. 2009032802 21600 7200 604800 3600"),
}

const testTransferZone = "miek.nl."

func TestTransfer(t *testing.T) {
	dns.HandleFunc(testTransferZone, func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
		r.Unpack()
		w.Hijack()

		env := make(chan *dns.Envelope)
		c := dns.NewClient()

		var wg sync.WaitGroup
		wg.Go(func() {
			err := c.TransferOut(w, r, env, nil)
			if err != nil {
				t.Fatal(err)
			}
			w.Close()
		})
		env <- &dns.Envelope{Answer: []dns.RR{testTransferData[0]}}
		env <- &dns.Envelope{Answer: []dns.RR{testTransferData[1]}}
		env <- &dns.Envelope{Answer: []dns.RR{testTransferData[2]}}
		env <- &dns.Envelope{Answer: []dns.RR{testTransferData[3]}}
		close(env)
	})
	defer dns.HandleRemove(testTransferZone)

	for _, name := range []string{"tcp", "tcp-tls", "tcp-ixfr", "tcp-tls-ixfr"} {
		t.Run(name, func(t *testing.T) {
			c := dns.NewClient()
			m := dns.NewMsg(testTransferZone, dns.TypeAXFR)
			ixfrsoa := []dns.RR{&dns.SOA{Hdr: *m.Question[0].Header(), Ns: ".", Mbox: ".", Serial: 2009032799}}
			addr := ""
			switch name {
			case "tcp", "tcp-ixfr":
				cancel, adr, _ := dnstest.TCPServer(":0")
				defer cancel()
				addr = adr
				if strings.HasSuffix(name, "-ixfr") {
					m = dns.NewMsg(testTransferZone, dns.TypeIXFR)
					m.Ns = ixfrsoa
				}
			case "tcp-tls", "tcp-tls-ixfr":
				cancel, adr, _ := dnstest.TLSServer(":0")
				defer cancel()
				addr = adr
				c.TLSConfig = dnstest.TLSConfig()
				if strings.HasSuffix(name, "-ixfr") {
					m = dns.NewMsg(testTransferZone, dns.TypeIXFR)
					m.Ns = ixfrsoa
				}
			}

			env, err := c.TransferIn(context.TODO(), m, addr, nil)
			if err != nil {
				t.Fatal("failed to setup zone transfer in", err)
			}

			i := 0
			for e := range env {
				if e.Error != nil {
					t.Errorf("unexpected error: %s", e.Error)
					break
				}
				i += len(e.Answer)
			}
			if i != len(testTransferData) {
				t.Fatalf("bad axfr: expected %d, got %d", len(testTransferData), i)
			}
		})
	}
}

// TestTransferEdgeCases checks that TransferIn detects the end of a transfer by itself, i.e. from the
// terminal SOA (AXFR) or the SOA countdown (IXFR), instead of relying on the server closing the
// connection. Each element of answers is sent as a separate message, so a client that misses the end
// reads on and returns too many RRs.
func TestTransferEdgeCases(t *testing.T) {
	testcases := []struct {
		name    string
		qtype   uint16
		answers [][]dns.RR
		serial  uint32
		wantErr bool
		want    int
	}{
		{"invalid", dns.TypeIXFR, [][]dns.RR{{}}, 2009032802, true, 0},
		{"not-soa", dns.TypeIXFR, [][]dns.RR{{testTransferData[1]}}, 2009032802, true, 0},
		{"single", dns.TypeIXFR, [][]dns.RR{{testTransferData[0]}}, 2009032800, false, 1},
		{
			"up-to-date", dns.TypeIXFR, [][]dns.RR{
				testTransferDataIncrementalData[:len(testTransferDataIncrementalData)/2],
				testTransferDataIncrementalData[len(testTransferDataIncrementalData)/2:],
			}, 2009032802, false, 1,
		},
		{
			// clientSerial (4200000000) is numerically > serverSerial (2009032802) but the
			// difference exceeds 2^31, so per RFC 1982 the client is behind and must receive
			// the full incremental transfer, not just the SOA.
			"rfc1982-behind", dns.TypeIXFR, [][]dns.RR{
				testTransferDataIncrementalData[:len(testTransferDataIncrementalData)/2],
				testTransferDataIncrementalData[len(testTransferDataIncrementalData)/2:],
			}, 4200000000, false, 8,
		},
		{"axfr-invalid", dns.TypeAXFR, [][]dns.RR{{}}, 0, true, 0},
		{"axfr-single", dns.TypeAXFR, [][]dns.RR{{testTransferData[0]}}, 0, false, 1},
		{
			// The entire zone in a single message, the opening and terminal SOA are in the same message.
			// The zone is sent twice, a client that misses the terminal SOA reads the second one too.
			"axfr-double-write", dns.TypeAXFR, [][]dns.RR{testTransferData, testTransferData}, 0, false, 4,
		},
		{
			"axfr-empty-zone", dns.TypeAXFR, [][]dns.RR{{testTransferData[0], testTransferData[3]}}, 0, false, 2,
		},
		{
			// The first message only holds the opening SOA, it must not be seen as the terminal one.
			"axfr-soa-only-first", dns.TypeAXFR, [][]dns.RR{testTransferData[:1], testTransferData[1:]}, 0, false, 4,
		},
		{
			"axfr-soa-only-last", dns.TypeAXFR, [][]dns.RR{testTransferData[:3], testTransferData[3:]}, 0, false, 4,
		},
		{
			// A server sends the complete IXFR diff inside a single TCP message. The client
			// must recognise the transfer as done and return without falling back to io.Copy
			// for a second read.
			"ixfr-double-write", dns.TypeIXFR, [][]dns.RR{testTransferDataIncrementalData, testTransferDataIncrementalData},
			2009032800, false, 8,
		},
	}

	var answers [][]dns.RR
	dns.HandleFunc(testTransferZone, func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
		r.Unpack()
		w.Hijack()

		env := make(chan *dns.Envelope)
		c := new(dns.Client)

		var wg sync.WaitGroup
		wg.Go(func() {
			c.TransferOut(w, r, env, nil)
			w.Close()
		})

		for _, ans := range answers {
			env <- &dns.Envelope{Answer: ans}
		}

		close(env)
	})
	defer dns.HandleRemove(testTransferZone)

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			answers = tc.answers
			cancel, addr, _ := dnstest.TCPServer(":0")
			defer cancel()

			c := new(dns.Client)
			m := dns.NewMsg(testTransferZone, tc.qtype)
			if tc.qtype == dns.TypeIXFR {
				m.Ns = []dns.RR{&dns.SOA{Hdr: *m.Question[0].Header(), Ns: ".", Mbox: ".", Serial: tc.serial}}
			}

			env, err := c.TransferIn(context.TODO(), m, addr, nil)
			if err != nil {
				t.Fatal("failed to zone transfer in", err)
			}

			var (
				gotErr error
				i      = 0
			)
			for e := range env {
				if e.Error != nil {
					gotErr = e.Error
				}
				i += len(e.Answer)
			}

			if gotErr == nil && tc.wantErr {
				t.Fatal("expected error, got none")
			}
			if gotErr != nil && !tc.wantErr {
				t.Fatalf("unexpected error: %s", gotErr)
			}
			if i != tc.want {
				t.Fatalf("bad transfer: expected %d, got %d", tc.want, i)
			}
		})
	}
}

func TestTransferIncremental(t *testing.T) {
	dns.HandleFunc(testTransferZone, func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
		r.Unpack()
		w.Hijack()

		env := make(chan *dns.Envelope)
		c := dns.NewClient()

		var wg sync.WaitGroup
		wg.Go(func() {
			err := c.TransferOut(w, r, env, nil)
			if err != nil {
				t.Fatal(err)
			}
			w.Close()
		})

		env <- &dns.Envelope{Answer: testTransferDataIncrementalData[:len(testTransferDataIncrementalData)/2]}
		env <- &dns.Envelope{Answer: testTransferDataIncrementalData[len(testTransferDataIncrementalData)/2:]}
		close(env)
	})
	defer dns.HandleRemove(testTransferZone)

	for _, name := range []string{"tcp", "tcp-tls"} {
		t.Run(name, func(t *testing.T) {
			c := dns.NewClient()
			m := dns.NewMsg(testTransferZone, dns.TypeIXFR)
			m.Ns = []dns.RR{&dns.SOA{Hdr: *m.Question[0].Header(), Ns: ".", Mbox: ".", Serial: 2009032800}}

			addr := ""
			switch name {
			case "tcp":
				cancel, adr, _ := dnstest.TCPServer(":0")
				defer cancel()
				addr = adr
			case "tcp-tls":
				cancel, adr, _ := dnstest.TLSServer(":0")
				defer cancel()
				addr = adr
				c.TLSConfig = dnstest.TLSConfig()
			}

			env, err := c.TransferIn(context.TODO(), m, addr, nil)
			if err != nil {
				t.Fatal("failed to setup zone transfer in", err)
			}

			i := 0
			for e := range env {
				if e.Error != nil {
					t.Fatalf("unexpected error: %s", e.Error)
				}
				i += len(e.Answer)
			}
			if i != len(testTransferDataIncrementalData) {
				t.Fatalf("bad ixfr: expected %d, got %d", len(testTransferDataIncrementalData), i)
			}
		})
	}
}

func TestTransferTSIG(t *testing.T) {
	dns.HandleFunc(testTransferZone, func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
		r.Unpack()
		w.Hijack()

		var wg sync.WaitGroup
		env := make(chan *dns.Envelope)
		c := dns.NewClient()
		options := &dns.TransferOption{TSIGSigner: dns.HmacTSIG{[]byte("geheim")}}
		wg.Go(func() {
			err := c.TransferOut(w, r, env, options)
			if err != nil {
				t.Fatal(err)
			}
			w.Close()
		})
		env <- &dns.Envelope{Answer: []dns.RR{testTransferData[0]}}
		env <- &dns.Envelope{Answer: []dns.RR{testTransferData[1]}}
		env <- &dns.Envelope{Answer: []dns.RR{testTransferData[2]}}
		env <- &dns.Envelope{Answer: []dns.RR{testTransferData[3]}}
		close(env)
	})
	defer dns.HandleRemove(testTransferZone)

	cancel, addr, _ := dnstest.TCPServer(":0")
	defer cancel()

	c := dns.NewClient()

	m := dns.NewMsg(testTransferZone, dns.TypeAXFR)
	m.Pseudo = []dns.RR{dns.NewTSIG(".", dns.HmacSHA512, 0)}

	options := &dns.TransferOption{TSIGSigner: dns.HmacTSIG{[]byte("geheim")}}
	env, err := c.TransferIn(context.TODO(), m, addr, options)
	if err != nil {
		t.Fatal("failed to zone transfer in", err)
	}

	for e := range env {
		if e.Error != nil {
			t.Fatal(e.Error)
		}
	}
}
