package service

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/authority"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func TestAuthorityQueryFallsBackToTCP(t *testing.T) {
	config := Config{Mode: "server", HTTPAddr: "127.0.0.1:0", DNSAddr: "127.0.0.1:0", StateDir: t.TempDir(), LocalLab: true, Environment: "lab", Zone: "windows.example.", KeyFile: serviceKey(t)}
	signer, err := window.LoadSigner(config.Zone, config.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	store, err := window.Open(config.StateDir, signer)
	if err != nil {
		t.Fatal(err)
	}
	inception := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	w, err := store.Create(window.Input{Content: strings.Repeat("x", 512), Inception: inception.Format(time.RFC3339), Expiration: inception.Add(time.Hour).Format(time.RFC3339)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	a, _ := startWindowService(t, config)
	q := dns.NewMsg(w.Name, dns.TypeTXT)
	q.Security = true
	q.UDPSize = 512
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := dns.Exchange(ctx, q, "udp", a.DNS)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Truncated || len(r.Answer) != 0 {
		t.Fatal("UDP does not require TCP fallback")
	}
	r, network, err := authority.Query(ctx, a.DNS, q)
	if err != nil {
		t.Fatal(err)
	}
	if network != "tcp" || r.Truncated || len(r.Answer) != 2 {
		t.Fatalf("fallback network=%s response=%s", network, r)
	}
	if r.Answer[1].String() != w.RRSIG {
		t.Fatal("TCP fallback changes the signature")
	}
	idle, err := net.DialTimeout("tcp", a.DNS, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = idle.Close() }()
	q.UDPSize = 1232
	q.Data = nil
	if _, err := dns.Exchange(ctx, q, "tcp", a.DNS); err != nil {
		t.Fatal("idle TCP connection blocks another client")
	}
}
