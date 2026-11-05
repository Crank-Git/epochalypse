package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/atomtest"
	"codeberg.org/miekg/dns/internal/dnsperf"
)

const Conffile = `
{
	dns {
		addr [::]:%s
	}
}

whoami.example.org {
    whoami
}
`

func TestAtomdnsPerf(t *testing.T) {
	const count = 8
	ports := [2]string{"8053", "8054"}
	dir := t.TempDir()
	conffile := dir + "/Conffile"

	for p, network := range []string{"udp", "tcp"} {
		os.WriteFile(conffile, fmt.Appendf(nil, Conffile, ports[p]), 0600)

		t.Run("atomdns-"+network, func(t *testing.T) {
			timeout := count*2*time.Second + 5*time.Second
			ctx, cancel := context.WithTimeout(context.Background(), timeout)

			if _, err := os.Stat("./atomdns"); err != nil {
				t.Skip("no atomdns binary found in .")
			}

			cmd := exec.CommandContext(ctx, "./atomdns", conffile)
			go func() {
				if err := cmd.Run(); err != nil {
					if _, ok := err.(*exec.ExitError); !ok {
						log.Fatal("no working atomdns binary found in .")
					}
				}
			}()

			queries := strings.NewReader("whoami.example.org. A")
			if err := dnsperf.Run(t, queries, fmt.Sprintf("127.0.0.1:%s", ports[p]), network, 2*time.Second, count); err != nil {
				t.Fatal(err)
			}
			t.Logf("canceled executing: %s", network)
			cancel()
		})
	}
}

func TestAtomdns(t *testing.T) {
	testcases := []struct {
		qname     string
		qtype     uint16
		qclass    uint16
		answerlen int
	}{
		{"www.example.org.", dns.TypeA, dns.ClassINET, 2},
		{"version.server.", dns.TypeTXT, dns.ClassCHAOS, 1},
		{"version.server.", dns.TypeTXT, dns.ClassINET, 0}, // should be refused
	}
	s, cancel, err := atomtest.New(`
10.0.0.0/24 {
    log
    whoami
}

example.org {
    log
    dbfile handlers/dbfile/zone/testdata/db.example.org {
        transfer
    }
}

version.server/CH {
	log
	chaos
}
`)
	defer cancel()
	if err != nil {
		t.Fatal(err)
	}

	c := new(dns.Client)
	for _, tc := range testcases {
		t.Run(tc.qname+"/"+dns.TypeToString[tc.qtype], func(t *testing.T) {
			m := dns.NewMsg(tc.qname, tc.qtype)
			m.Question[0].Header().Class = tc.qclass

			r, _, err := c.Exchange(context.TODO(), m, "udp", s.Addr()[0])
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Answer) != tc.answerlen {
				t.Fatalf("expected %d answers, got %d", tc.answerlen, len(r.Answer))
			}
		})
	}

	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range testcases {
		t.Run("reload/"+tc.qname+"/"+dns.TypeToString[tc.qtype], func(t *testing.T) {
			m := dns.NewMsg(tc.qname, tc.qtype)
			m.Question[0].Header().Class = tc.qclass

			r, _, err := c.Exchange(context.TODO(), m, "udp", s.Addr()[0])
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Answer) != tc.answerlen {
				t.Fatalf("expected %d answers, got %d", tc.answerlen, len(r.Answer))
			}
		})
	}
}

func TestAtomdnsCompliance_RFC8906(t *testing.T) {
	const zone = "example.org."
	testcases := []struct {
		name string
		in   *dns.Msg
		cmp  func(r *dns.Msg) error
	}{
		{
			"8.1.1.configured",
			func() *dns.Msg {
				m := dns.NewMsg(zone, dns.TypeSOA)
				m.RecursionDesired = false
				return m
			}(),
			func(r *dns.Msg) error {
				if r.Rcode != dns.RcodeSuccess {
					return fmt.Errorf("expected NOERROR")
				}
				if !r.Authoritative {
					return fmt.Errorf("expected AA bit to be true")
				}
				if r.RecursionDesired {
					return fmt.Errorf("expected RD bit to be false")
				}
				if r.AuthenticatedData {
					return fmt.Errorf("expected AD bit to be false")
				}
				if len(r.Answer) != 1 {
					return fmt.Errorf("expected record in answer section, got %d records", len(r.Answer))
				}
				if len(r.Pseudo) > 0 {
					return fmt.Errorf("expected no pseudo section, got %d records", len(r.Pseudo))
				}

				return nil
			},
		},
		{
			"8.1.1.unknowntype",
			func() *dns.Msg {
				m := dns.NewMsg(zone, dns.TypeSOA)
				m.Question[0] = &dns.RFC3597{
					Hdr:    dns.Header{Name: zone, Class: dns.ClassINET},
					RRType: 1000,
				}
				m.RecursionDesired = false
				return m
			}(),
			func(r *dns.Msg) error {
				if r.Rcode != dns.RcodeSuccess {
					return fmt.Errorf("expected NOERROR")
				}
				if !r.Authoritative {
					return fmt.Errorf("expected AA bit to be true")
				}
				if r.RecursionDesired {
					return fmt.Errorf("expected RD bit to be false")
				}
				if r.AuthenticatedData {
					return fmt.Errorf("expected AD bit to be false")
				}
				if len(r.Answer) > 0 {
					return fmt.Errorf("expected no answer section, got %d records", len(r.Answer))
				}
				if len(r.Pseudo) > 0 {
					return fmt.Errorf("expected no pseudo section, got %d records", len(r.Pseudo))
				}

				return nil
			},
		},
		{
			"8.1.3.1.cd=1", // we know we reply
			func() *dns.Msg { return nil }(),
			func(r *dns.Msg) error { return nil },
		},
		{
			"8.1.3.2.ad=1", // we know we reply
			func() *dns.Msg { return nil }(),
			func(r *dns.Msg) error { return nil },
		},
		{
			"8.1.3.4.rd=1",
			func() *dns.Msg {
				m := dns.NewMsg(zone, dns.TypeSOA)
				m.RecursionDesired = true
				return m
			}(),
			func(r *dns.Msg) error {
				if r.Rcode != dns.RcodeSuccess {
					return fmt.Errorf("expected NOERROR")
				}
				if !r.Authoritative {
					return fmt.Errorf("expected AA bit to be true")
				}
				if !r.RecursionDesired {
					return fmt.Errorf("expected RD bit to be true")
				}
				if r.AuthenticatedData {
					return fmt.Errorf("expected AD bit to be false")
				}
				if r.Zero {
					return fmt.Errorf("expected Z bit to be false")
				}
				if len(r.Answer) != 1 {
					return fmt.Errorf("expected record in answer section, got %d records", len(r.Answer))
				}
				if len(r.Pseudo) > 0 {
					return fmt.Errorf("expected no pseudo section, got %d records", len(r.Pseudo))
				}

				return nil
			},
		},
		{
			"8.1.4.unknownopcode=1",
			func() *dns.Msg {
				m := dns.NewMsg(zone, dns.TypeSOA)
				m.Opcode = 15
				return m
			}(),
			func(r *dns.Msg) error {
				if r.Rcode != dns.RcodeNotImplemented {
					return fmt.Errorf("expected NOTIMP")
				}
				if r.Authoritative {
					return fmt.Errorf("expected AA bit to be false")
				}
				if r.RecursionDesired {
					return fmt.Errorf("expected RD bit to be false")
				}
				if r.AuthenticatedData {
					return fmt.Errorf("expected AD bit to be false")
				}
				if len(r.Answer) > 0 {
					return fmt.Errorf("expected no answer section, got %d records", len(r.Answer))
				}
				if len(r.Ns) > 0 {
					return fmt.Errorf("expected no ns section, got %d records", len(r.Ns))
				}
				if len(r.Extra) > 0 {
					return fmt.Errorf("expected no extra section, got %d records", len(r.Extra))
				}
				if len(r.Pseudo) > 0 {
					return fmt.Errorf("expected no pseudo section, got %d records", len(r.Pseudo))
				}

				return nil
			},
		},
		{
			"8.1.5.tcp", // we know we reply
			func() *dns.Msg { return nil }(),
			func(r *dns.Msg) error { return nil },
		},
	}

	s, cancel, err := atomtest.New(`
example.org {
    dbfile handlers/dbfile/zone/testdata/db.example.org
}
`)
	defer cancel()
	if err != nil {
		t.Fatal(err)
	}

	c := new(dns.Client)
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.in == nil {
				t.SkipNow()
			}
			r, _, err := c.Exchange(context.TODO(), tc.in, "udp", s.Addr()[0])
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.cmp(r); err != nil {
				t.Logf("%s\n", r)
				t.Fatalf("expected no error, got %s", err)
			}
		})
	}
}
