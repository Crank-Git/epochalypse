package acl

import (
	"context"
	"net/netip"
	"slices"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnsctx"
	"codeberg.org/miekg/dns/dnsutil"
	"github.com/phemmer/go-iptrie"
)

// rule defines ACL policies which will be enforced.
type rule struct {
	policies []policy
}

// policy defines the ACL policy for DNS messages.
// A policy performs the specified action (block/allow) on all DNS messages matched by source IP or QTYPE.
type policy struct {
	action dns.MsgAcceptAction

	// One of these is non-nil and carries the policy
	net *policyNet
	ctx *policyCtx
}

type policyNet struct {
	qtypes  []uint16
	opcodes []uint8
	filter  *iptrie.Trie
}

type policyCtx struct {
	ctx    string
	values []any
}

const MsgFilter = dns.MsgAcceptAction(10)

// msgNoMatch is returned by match when none of the policies matched, the next rule must then be tried.
const msgNoMatch = dns.MsgAcceptAction(11)

// match matches the DNS message with a list of ACL polices and returns suitable action against the message.
// If no policy matches msgNoMatch is returned.
func match(ctx context.Context, policies []policy, w dns.ResponseWriter, r *dns.Msg) dns.MsgAcceptAction {
	for _, policy := range policies {
		switch {
		case policy.net != nil:
			ip, err := netip.ParseAddr(dnsutil.RemoteIP(w))
			if err != nil {
				// remote is not add network address (unix socket?), continue
				continue
			}
			if x := dnsctx.Addr(ctx, "ecs/addr"); x.IsValid() {
				ip = x
			}

			matchAll := len(policy.net.opcodes) == 0
			match := slices.Contains(policy.net.opcodes, r.Opcode)
			if !matchAll && !match {
				continue
			}

			_, qtype := dnsutil.Question(r)
			matchAll = len(policy.net.qtypes) == 0
			match = slices.Contains(policy.net.qtypes, qtype)
			if !matchAll && !match {
				continue
			}

			if !policy.net.filter.Contains(ip) {
				continue
			}
			return policy.action
		case policy.ctx != nil:
			if dnsctx.Match(ctx, policy.ctx.ctx, policy.ctx.values) {
				return policy.action
			}
		}
	}
	return msgNoMatch
}
