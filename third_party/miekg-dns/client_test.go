package dns_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnstest"
	"codeberg.org/miekg/dns/dnsutil"
)

func ExampleClient_Exchange() {
	m := dns.NewMsg("www.example.org", dns.TypeA)
	r, err := dns.Exchange(context.TODO(), m, "udp", "8.8.8.8:53")
	if err != nil {
		log.Printf("Failed to exchange: %v", err)
		return
	}
	for _, rr := range r.Answer {
		if a, ok := rr.(*dns.A); ok {
			fmt.Println(a.A)
		}
	}
}

func ExampleClient_Exchange_nxdomain() {
	m := dns.NewMsg("wwww.example.org", dns.TypeA)
	r, err := dns.Exchange(context.TODO(), m, "udp", "8.8.8.8:53")
	if err != nil {
		log.Printf("Failed to exchange: %v", err)
		return
	}
	if m.Rcode != dns.RcodeNameError {
		log.Printf("Expected NXDOMAIN, got %s", dns.RcodeToString[m.Rcode])
		return
	}
	// Authority section should contain the SOA record.
	for _, rr := range r.Ns {
		if soa, ok := rr.(*dns.SOA); ok {
			fmt.Println(soa.Serial)
		}
	}
}

// ExampleClient_Exchange_edns0 shows how to add an EDNS0 option to a message. See [dns.NSID].
func ExampleClient_Exchange_edns0() {
	m := dns.NewMsg("wwww.example.org", dns.TypeA)
	m.Pseudo = append(m.Pseudo, &dns.NSID{}) // we ask the server to put the server id in the reply.
	r, err := dns.Exchange(context.TODO(), m, "udp", "8.8.8.8:53")
	if err != nil {
		log.Printf("Failed to exchange: %v", err)
		return
	}
	for _, rr := range r.Pseudo {
		if nsid, ok := rr.(*dns.NSID); ok {
			fmt.Printf("NSID returned from server: %s\n", nsid.Nsid)
			break
		}
	}
}

func delayHandler(ctx context.Context, w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	dnsutil.SetReply(m, req)
	m.Extra = []dns.RR{&dns.TXT{Hdr: dns.Header{Name: m.Question[0].Header().Name, Class: dns.ClassINET}, Txt: []string{"Hello world"}}}
	time.Sleep(time.Duration(rand.Intn(5)) * 10 * time.Millisecond)
	m.Pack()
	io.Copy(w, m)
}

func TestClient_TCP(t *testing.T) {
	dns.HandleFunc("example.org.", delayHandler)
	defer dns.HandleRemove("example.org.")
	cancel, addr, err := dnstest.TCPServer(":0")
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	const messages = 40
	c := dns.NewClient()
	errs := make(chan error, messages)
	wg := new(sync.WaitGroup)
	for range messages {
		wg.Go(func() {
			m := dns.NewMsg("www.example.org.", dns.TypeA)
			m.ID = dns.ID()
			m.Pack()
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				errs <- err
			}

			_, _, err = c.ExchangeWithConn(context.TODO(), m, conn)
			conn.Close()
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	i := 0
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
		i++
	}
	if i != messages {
		t.Fatalf("expected %d responses, got %d", messages, i)
	}
	if err := c.Err(); err != nil {
		t.Fatal(err)
	}
}

func fakeHandler(ctx context.Context, w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	dnsutil.SetReply(m, req)
	m.ID = 5
	m.Extra = []dns.RR{&dns.TXT{Hdr: dns.Header{Name: m.Question[0].Header().Name, Class: dns.ClassINET}, Txt: []string{"Hello world"}}}
	m.Pack()
	io.Copy(w, m)
}

func TestClient_ID(t *testing.T) {
	dns.HandleFunc("example.org.", fakeHandler)
	defer dns.HandleRemove("example.org.")
	cancel, addr, err := dnstest.Server(":0")
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	m := dns.NewMsg("www.example.org.", dns.TypeA)

	_, err = dns.Exchange(context.TODO(), m, "udp", addr)
	if err == nil {
		t.Fatal("expected error, got none")
	}
	if errtxt := err.Error(); !strings.Contains(errtxt, "does not match") {
		t.Fatalf("expected ID error, got %s", errtxt)
	}
}
