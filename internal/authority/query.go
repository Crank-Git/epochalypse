package authority

import (
	"codeberg.org/miekg/dns"
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// Query reads one DNS response through UDP with explicit TCP fallback.
func Query(ctx context.Context, address string, request *dns.Msg) (*dns.Msg, string, error) {
	if request == nil || len(request.Question) != 1 {
		return nil, "", errors.New("provide one DNS question")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	query := request.Copy()
	query.Data = nil
	r, err := exchange(ctx, query, "udp", address)
	if err != nil {
		return nil, "udp", fmt.Errorf("query authority over UDP: %w", err)
	}
	if !r.Truncated {
		return r, "udp", nil
	}
	r, err = exchange(ctx, query, "tcp", address)
	if err != nil {
		return nil, "tcp", fmt.Errorf("query authority over TCP: %w", err)
	}
	return r, "tcp", nil
}

func exchange(ctx context.Context, query *dns.Msg, network, address string) (*dns.Msg, error) {
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	defer func() { _ = connection.Close() }()
	// Query supplies one absolute deadline for both transports.
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return nil, err
	}
	// The adapter replaces read deadlines and does not cancel blocked reads.
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	response, _, err := dns.NewClient().ExchangeWithConn(ctx, query, connection)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil && !time.Now().Before(deadline) {
		return nil, context.DeadlineExceeded
	}
	return response, err
}
