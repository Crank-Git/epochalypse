package service

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

type dnsListeners struct {
	tcp net.Listener
	udp net.PacketConn
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

func (d *dnsListeners) close() { _ = d.tcp.Close(); _ = d.udp.Close() }

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
		if response := refused(buffer[:n]); response != nil {
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
		// A fixed deadline bounds each scaffold connection until the DNS feature supplies concurrent service.
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		serveDNSConnection(conn)
		_ = conn.Close()
	}
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
