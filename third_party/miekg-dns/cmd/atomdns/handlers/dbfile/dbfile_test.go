package dbfile_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/atomtest"
	"codeberg.org/miekg/dns/dnstest"
)

func TestDbfile(t *testing.T) {
	testcases := []struct {
		name     string
		question string
		answer   []dns.RR
	}{
		{
			"outofzonecname",
			"shop.miek.nl.",
			[]dns.RR{
				dnstest.New("shop.miek.nl. IN CNAME example.com."),
			},
		},
	}
	conf := `miek.nl {
		dbfile zone/testdata/db.miek.nl
	}`

	server, cancel, err := atomtest.New(conf)
	if err != nil {
		t.Fatal(err)
	}
	addr := server.Addr()[0]
	defer cancel()

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {

			c := new(dns.Client)
			m := dns.NewMsg(tc.question, dns.TypeA)
			r, _, err := c.Exchange(context.TODO(), m, "udp", addr)
			if err != nil {
				t.Fatal(err)
			}
			for i, a := range tc.answer {
				if dns.Compare(a, r.Answer[i]) != 0 {
					t.Fatalf("expected %s and %s to be equal", a, r.Answer[i])
				}
			}
		})
	}
}

func TestDbfileTransferOut(t *testing.T) {
	testcases := []struct {
		name    string
		input   string
		success bool
	}{
		{
			"axfr",
			`example.org {
				dbfile zone/testdata/db.example.org {
					transfer
			    }
			}`, true,
		},
		{
			"no-axfr",
			`example.org {
				dbfile zone/testdata/db.example.org
			}`, false,
		},
		{
			"axfr",
			`example.org {
				dbfile zone/testdata/db.example.org {
				 	transfer {
						to {
							notify 127.0.0.1
						}
					}
				}
			}`, false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			server, cancel, err := atomtest.New(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			defer cancel()

			c := new(dns.Client)
			addrs := server.Addr()
			m := dns.NewMsg("example.org.", dns.TypeAXFR)
			env, err := c.TransferIn(context.TODO(), m, addrs[1], nil)
			if err != nil {
				if !tc.success {
					return
				}
				t.Fatalf("failed to setup zone transfer in: %s", err)
			}

			i := 0 // expect at least more then 1 record, last one should be SOA.
			var last dns.RR
			for e := range env {
				if e.Error != nil {
					if tc.success {
						t.Fatalf("got unexpected error: %s", e.Error)
					}
					return
				}
				last = e.Answer[len(e.Answer)-1]
				i++
			}
			if i == 0 {
				t.Fatal("expected more than 0 records")
			}
			if _, ok := last.(*dns.SOA); !ok {
				t.Fatal("last record should be SOA")
			}
		})
	}
}

func TestDbfileTransferIn(t *testing.T) {
	// This runs 2 server where one server transfers out, and the other one transfers in. The test is written
	// to test the latter.
	config := `example.org {
				dbfile zone/testdata/db.example.org {
					transfer
			    }
			}`

	primary, cancel1, err := atomtest.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel1()
	addr := primary.Addr()
	config = fmt.Sprintf(`example.org {
				dbfile db.example.org.transferred {
					transfer {
						from %s
					}
				}
			}`, addr[1])
	_, cancel2, err := atomtest.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel2()
	// wait for db.example.org.transferred to exist
	for {
		_, err := os.Stat("db.example.org.transferred")
		if err == nil {
			break
		}
		time.Sleep(100 * time.Microsecond)
	}
	if err := os.Remove("db.example.org.transferred"); err != nil {
		t.Fatal("failed to remove file that should exist")
	}
}
