package service

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/owner"
)

func TestAuthorityClosesPersistentTCPConnectionsAtShutdown(t *testing.T) {
	config := Config{Mode: "server", HTTPAddr: "127.0.0.1:0", DNSAddr: "127.0.0.1:0", StateDir: t.TempDir(), LocalLab: true, Environment: "lab", Zone: "windows.example.", KeyFile: serviceKey(t)}
	a, stop := startWindowService(t, config)
	conn, err := net.DialTimeout("tcp", a.DNS, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		q := dns.NewMsg(config.Zone, dns.TypeDNSKEY)
		q.Security = true
		q.UDPSize = 1232
		if err := q.Pack(); err != nil {
			t.Fatal(err)
		}
		var size [2]byte
		binary.BigEndian.PutUint16(size[:], uint16(len(q.Data))) // #nosec G115 -- The fixed DNSKEY question is smaller than 65536 bytes.
		if _, err := conn.Write(append(size[:], q.Data...)); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(conn, size[:]); err != nil {
			t.Fatal(err)
		}
		r := &dns.Msg{Data: make([]byte, int(binary.BigEndian.Uint16(size[:])))}
		if _, err := io.ReadFull(conn, r.Data); err != nil {
			t.Fatal(err)
		}
		if err := r.Unpack(); err != nil {
			t.Fatal(err)
		}
		if len(r.Answer) != 2 {
			t.Fatal("persistent TCP query loses signed DNSKEY")
		}
	}
	stop()
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("shutdown retains a live TCP connection")
	}
}

func TestAuthorityBoundsAcceptedTCPConnections(t *testing.T) {
	d, err := openDNS(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.close)
	d.respond = func(_ []byte, _ bool) []byte { return nil }
	for range 64 {
		client, server := net.Pipe()
		t.Cleanup(func() { _ = client.Close() })
		if !d.track(server) {
			t.Fatal("connection limit rejects an available slot")
		}
		go d.serveAuthorityConnection(server)
	}
	client, server := net.Pipe()
	defer func() { _ = client.Close(); _ = server.Close() }()
	if d.track(server) {
		t.Fatal("authority accepts a connection beyond its limit")
	}
	d.close()
	if d.track(server) {
		t.Fatal("closed authority accepts a connection")
	}
}

func TestAuthorityRejectsIncompleteFramesAndInvalidResponses(t *testing.T) {
	for _, tc := range []struct{ payload, response []byte }{{nil, nil}, {[]byte{0}, nil}, {[]byte{0, 12, 1}, nil}, {[]byte{0, 0}, nil}, {[]byte{0, 0}, make([]byte, 65536)}} {
		client, server := net.Pipe()
		d := dnsListeners{respond: func(_ []byte, _ bool) []byte { return tc.response }}
		done := make(chan error, 1)
		go func() { done <- d.authorityFrame(server); _ = server.Close() }()
		if len(tc.payload) > 0 {
			_, _ = client.Write(tc.payload)
		}
		_ = client.Close()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("invalid frame passes")
			}
		case <-time.After(time.Second):
			t.Fatal("invalid authority frame blocks")
		}
	}
}

func TestMainRejectsAnExplicitLabKeyOutsideTheLab(t *testing.T) {
	key := serviceKey(t)
	// #nosec G304 -- The test creates this synthetic key file.
	data, err := os.ReadFile(key)
	if err != nil {
		t.Fatal(err)
	} // #nosec G304 -- The test creates this synthetic file.
	data = append(data, []byte("Comment: epochalypse-lab-only\n")...)
	if err := os.WriteFile(key, data, 0600); err != nil {
		t.Fatal(err)
	} // #nosec G703 -- The test creates this synthetic file.
	state := t.TempDir()
	if s, err := openMainState(Config{StateDir: state, Zone: "windows.example.", KeyFile: key}); err == nil {
		s.close()
		t.Fatal("production accepts a marked lab key")
	} else if !strings.Contains(err.Error(), "rejects an explicit lab key") {
		t.Fatal(err)
	}
	owners, err := owner.Open(state)
	if err != nil {
		t.Fatal("failed lab-key check retains the owner lock")
	}
	if err := owners.Close(); err != nil {
		t.Fatal(err)
	}
}
