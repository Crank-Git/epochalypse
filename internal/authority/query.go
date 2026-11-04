package authority

import (
	"codeberg.org/miekg/dns"
	"context"
	"errors"
	"fmt"
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
	r, err := dns.Exchange(ctx, query, "udp", address)
	if err != nil {
		return nil, "udp", fmt.Errorf("query authority over UDP: %w", err)
	}
	if !r.Truncated {
		return r, "udp", nil
	}
	r, err = dns.Exchange(ctx, query, "tcp", address)
	if err != nil {
		return nil, "tcp", fmt.Errorf("query authority over TCP: %w", err)
	}
	return r, "tcp", nil
}
