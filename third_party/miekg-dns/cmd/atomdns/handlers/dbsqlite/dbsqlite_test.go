package dbsqlite_test

import (
	"context"
	"strings"
	"testing"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/atomtest"
)

func TestDbsqlite(t *testing.T) {
	testcases := []struct {
		name  string
		input string
		exp   string
	}{
		{
			"query",
			`example.org {
				dbsqlite testdata/example.db
			}`,
			"127.0.0.1",
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
			m := dns.NewMsg("www.example.org.", dns.TypeA)
			resp, _, err := c.Exchange(context.TODO(), m, "udp", addrs[0])
			if err != nil {
				t.Fatalf("failed to send query: %s", err)
			}
			if !strings.Contains(resp.String(), tc.exp) {
				t.Fatalf("expected %s to be contained in %s", tc.exp, resp.String())
			}
		})
	}
}
