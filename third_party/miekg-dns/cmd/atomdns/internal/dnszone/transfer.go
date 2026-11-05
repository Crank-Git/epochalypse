package dnszone

import (
	"context"
	"fmt"
	"sync"

	"codeberg.org/miekg/dns"
)

func TransferOut(z Interface, ctx context.Context, w dns.ResponseWriter, r *dns.Msg) error {
	apex := z.Apex()
	if apex == nil {
		return fmt.Errorf("waiting for zone '%s' to appear", z.Origin())
	}

	w.Hijack()
	env := make(chan *dns.Envelope)
	c := dns.NewClient()
	var wg sync.WaitGroup

	i := 0
	ch := make(chan error)
	ctx, cancel := context.WithCancel(ctx)
	wg.Go(func() {
		err := c.TransferOut(w, r, env, nil)
		w.Close()
		cancel()
		ch <- err
	})

	z.Walk(func(n *Node) bool {
		if len(n.RRs) == 0 { // skip empty non-terminals
			return true
		}
		select {
		case env <- &dns.Envelope{Answer: n.RRs}:
			i++
			return true
		case <-ctx.Done():
			return false
		}
	})
	for _, rr := range apex.RRs {
		if s, ok := rr.(*dns.SOA); ok {
			i++
			env <- &dns.Envelope{Answer: []dns.RR{s}}
		}
	}

	close(env)
	err := <-ch
	if i == 0 {
		return fmt.Errorf("no RRs transferred")
	}
	return err
}
