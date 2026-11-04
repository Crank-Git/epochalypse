package service

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestTCPRefusesLargeFrames(t *testing.T) {
	for _, length := range []int{513, 1232, 65535} {
		client, server := net.Pipe()
		done := make(chan struct{})
		go func() { serveDNSConnection(server); _ = server.Close(); close(done) }()
		if err := client.SetDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		frame := make([]byte, length+2)
		binary.BigEndian.PutUint16(frame[:2], uint16(length))
		frame[2], frame[3], frame[4] = 0x12, 0x34, 1
		if _, err := client.Write(frame); err != nil {
			t.Fatal(err)
		}
		response := make([]byte, 14)
		if _, err := io.ReadFull(client, response); err != nil {
			t.Fatal(err)
		}
		if binary.BigEndian.Uint16(response[:2]) != 12 || response[2] != 0x12 || response[3] != 0x34 || response[5] != 5 {
			t.Fatalf("response: %v", response)
		}
		_ = client.Close()
		<-done
	}
}

type completeDatagram struct {
	net.PacketConn
	t *testing.T
}

func (c completeDatagram) ReadFrom(buffer []byte) (int, net.Addr, error) {
	if len(buffer) < 65535 {
		c.t.Errorf("receive buffer truncates standard datagrams: %d bytes", len(buffer))
	}
	return 0, nil, net.ErrClosed
}

func TestUDPReadsCompleteDatagrams(t *testing.T) {
	d := dnsListeners{udp: completeDatagram{t: t}}
	if err := d.serveUDP(); err != net.ErrClosed {
		t.Fatal(err)
	}
}

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
