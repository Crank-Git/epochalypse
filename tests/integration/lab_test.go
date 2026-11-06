package integration

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/service"
)

func TestLabStartsFiveListeners(t *testing.T) {
	for _, mode := range []string{"server", "dns", "agent", "agent", "agent"} {
		ctx, cancel := context.WithCancel(context.Background())
		ready := make(chan service.Addresses, 1)
		done := make(chan error, 1)
		config := service.Config{Mode: mode, HTTPAddr: "127.0.0.1:0", DNSAddr: "127.0.0.1:0", StateDir: t.TempDir(), AgentID: "lab-agent"}
		go func() { done <- service.Serve(ctx, config, func(a service.Addresses) { ready <- a }) }()
		t.Cleanup(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(5 * time.Second):
				t.Error("shutdown timeout")
			}
		})
		var address service.Addresses
		select {
		case address = <-ready:
		case <-time.After(5 * time.Second):
			t.Fatal("start timeout")
		}
		client := &http.Client{Timeout: time.Second}
		response, err := client.Get("http://" + address.HTTP + "/healthz")
		if err != nil {
			t.Fatal(err)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s health = %d", mode, response.StatusCode)
		}
		if mode == "dns" {
			checkDNS(t, address.DNS)
		}
	}
}

func checkDNS(t *testing.T, address string) {
	t.Helper()
	query := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 1}
	for _, network := range []string{"udp", "tcp"} {
		conn, err := net.DialTimeout(network, address, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		payload := query
		if network == "tcp" {
			payload = append([]byte{0, 17}, query...)
		}
		if _, err := conn.Write(payload); err != nil {
			t.Fatal(err)
		}
		if network == "tcp" {
			var size [2]byte
			if _, err := io.ReadFull(conn, size[:]); err != nil {
				t.Fatal(err)
			}
			if binary.BigEndian.Uint16(size[:]) != 12 {
				t.Fatalf("DNS frame length: %v", size)
			}
		}
		response := make([]byte, 12)
		if _, err := io.ReadFull(conn, response); err != nil {
			t.Fatal(err)
		}
		if response[0] != query[0] || response[1] != query[1] || response[2]&0x80 == 0 || response[3]&15 != 5 {
			t.Fatalf("DNS %s response: %v", network, response)
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
