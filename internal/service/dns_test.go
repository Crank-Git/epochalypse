package service

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestDNSRejectsMalformedQueries(t *testing.T) {
	for _, query := range [][]byte{nil, make([]byte, 11), {0, 0, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0}} {
		if response := refused(query); response != nil {
			t.Fatalf("accepted %v", query)
		}
	}
}

func TestTCPRejectsInvalidFrames(t *testing.T) {
	for _, payload := range [][]byte{nil, {2, 1}, {0, 12, 0}, {0, 1, 0}} {
		client, server := net.Pipe()
		done := make(chan struct{})
		go func() { serveDNSConnection(server); _ = server.Close(); close(done) }()
		if err := client.SetDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if len(payload) > 0 {
			if _, err := client.Write(payload); err != nil {
				t.Fatal(err)
			}
		}
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("invalid frame blocks connection")
		}
	}
}

func TestTCPRefusesValidQueries(t *testing.T) {
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() { serveDNSConnection(server); _ = server.Close(); close(done) }()
	if err := client.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(append([]byte{0, 12}, make([]byte, 12)...)); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 14)
	if _, err := client.Read(response); err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint16(response[:2]) != 12 || response[5] != 5 {
		t.Fatalf("response: %v", response)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	<-done
}
