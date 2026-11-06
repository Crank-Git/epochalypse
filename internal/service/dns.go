package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type dnsListeners struct {
	tcp         net.Listener
	udp         net.PacketConn
	respond     func([]byte, bool) []byte
	mu          sync.Mutex
	connections map[net.Conn]struct{}
	closing     bool
	workers     sync.WaitGroup
}

// The TCP length field limits allocation and covers every standard UDP datagram.
const maxDNSMessage = 1<<16 - 1

func openDNS(ctx context.Context, address string) (*dnsListeners, error) {
	var lc net.ListenConfig
	tcp, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("bind TCP DNS address: %w", err)
	}
	udp, err := lc.ListenPacket(ctx, "udp", tcp.Addr().String())
	if err != nil {
		_ = tcp.Close()
		return nil, fmt.Errorf("bind UDP DNS address: %w", err)
	}
	return &dnsListeners{tcp: tcp, udp: udp}, nil
}

func (d *dnsListeners) close() {
	d.mu.Lock()
	d.closing = true
	_ = d.tcp.Close()
	for conn := range d.connections {
		_ = conn.Close()
	}
	d.mu.Unlock()
	_ = d.udp.Close()
	d.workers.Wait()
}

// DNSSEC and record service belong to the DNS feature. Refuse every lab query.
func refused(query []byte) []byte {
	if len(query) < 12 || query[2]&0x80 != 0 {
		return nil
	}
	response := make([]byte, 12)
	copy(response, query[:2])
	response[2] = 0x80 | query[2]&0x79
	response[3] = 5
	return response
}

func (d *dnsListeners) serveUDP() error {
	buffer := make([]byte, maxDNSMessage)
	for {
		n, peer, err := d.udp.ReadFrom(buffer)
		if err != nil {
			return err
		}
		response := refused(buffer[:n])
		if d.respond != nil {
			response = d.respond(buffer[:n], true)
		}
		if response != nil {
			if _, err := d.udp.WriteTo(response, peer); err != nil {
				return err
			}
		}
	}
}

func (d *dnsListeners) serveTCP() error {
	for {
		conn, err := d.tcp.Accept()
		if err != nil {
			return err
		}
		if d.respond != nil {
			if !d.track(conn) {
				_ = conn.Close()
				continue
			}
			go d.serveAuthorityConnection(conn)
			continue
		}
		// A fixed deadline bounds each scaffold connection until the DNS feature supplies concurrent service.
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		serveDNSConnection(conn)
		_ = conn.Close()
	}
}

func (d *dnsListeners) track(conn net.Conn) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closing || len(d.connections) >= 64 {
		return false
	}
	if d.connections == nil {
		d.connections = make(map[net.Conn]struct{})
	}
	d.connections[conn] = struct{}{}
	d.workers.Add(1)
	return true
}

func (d *dnsListeners) serveAuthorityConnection(conn net.Conn) {
	defer func() {
		_ = conn.Close()
		d.mu.Lock()
		delete(d.connections, conn)
		d.mu.Unlock()
		d.workers.Done()
	}()
	for range 64 {
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		if err := d.authorityFrame(conn); err != nil {
			return
		}
	}
}

func (d *dnsListeners) authorityFrame(conn net.Conn) error {
	var size [2]byte
	if _, err := io.ReadFull(conn, size[:]); err != nil {
		return err
	}
	query := make([]byte, int(binary.BigEndian.Uint16(size[:])))
	if _, err := io.ReadFull(conn, query); err != nil {
		return err
	}
	response := d.respond(query, false)
	if len(response) == 0 || len(response) > maxDNSMessage {
		return fmt.Errorf("invalid authority response length")
	}
	binary.BigEndian.PutUint16(size[:], uint16(len(response))) // #nosec G115 -- The response length is bounded above.
	_, err := io.Copy(conn, bytes.NewReader(append(size[:], response...)))
	return err
}

func serveDNSConnection(conn net.Conn) {
	var size [2]byte
	if _, err := io.ReadFull(conn, size[:]); err != nil {
		return
	}
	length := binary.BigEndian.Uint16(size[:])
	query := make([]byte, int(length))
	if _, err := io.ReadFull(conn, query); err != nil {
		return
	}
	response := refused(query)
	if response == nil {
		return
	}
	binary.BigEndian.PutUint16(size[:], 12)
	_, _ = conn.Write(append(size[:], response...))
}
