package authority

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
)

func silentQueryTarget(t *testing.T, fallback bool) (string, <-chan struct{}) {
	t.Helper()
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udp, err := net.ListenPacket("udp", tcp.Addr().String())
	if err != nil {
		_ = tcp.Close()
		t.Fatal(err)
	}
	ready, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release); _ = udp.Close(); _ = tcp.Close(); <-done })
	go func() {
		defer close(done)
		packet := make([]byte, 1232)
		n, peer, err := udp.ReadFrom(packet)
		if err != nil {
			return
		}
		if !fallback {
			close(ready)
			return
		}
		packet = packet[:n]
		packet[2] |= 0x82
		if _, err := udp.WriteTo(packet, peer); err != nil {
			return
		}
		connection, err := tcp.Accept()
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		if connection.SetReadDeadline(time.Now().Add(time.Second)) != nil {
			return
		}
		var frame [2]byte
		if _, err := io.ReadFull(connection, frame[:]); err != nil {
			return
		}
		if _, err := io.CopyN(io.Discard, connection, int64(binary.BigEndian.Uint16(frame[:]))); err != nil {
			return
		}
		close(ready)
		<-release
	}()
	return tcp.Addr().String(), ready
}

func TestQueryCancellationClosesBlockedUDPAndTCPReads(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "udp", true: "tcp"}[fallback], func(t *testing.T) {
			address, ready := silentQueryTarget(t, fallback)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, _, err := Query(ctx, address, dns.NewMsg("windows.example.", dns.TypeTXT)); done <- err }()
			select {
			case <-ready:
			case <-time.After(time.Second):
				t.Fatal("the query does not reach the blocked read")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation loses its cause: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation does not close the blocked read")
			}
		})
	}
}

func TestQueryKeepsTheCallerDeadlineAcrossTCPFallback(t *testing.T) {
	address, ready := silentQueryTarget(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := Query(ctx, address, dns.NewMsg("windows.example.", dns.TypeTXT)); done <- err }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("the query does not reach TCP fallback")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("fallback loses the caller deadline: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("TCP fallback replaces the earlier caller deadline")
	}
}
