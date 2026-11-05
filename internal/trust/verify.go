// Package trust verifies explicit public anchors for the experimental authority.
package trust

import (
	"context"
	"errors"
	"math"
	"net"
	"strings"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/authority"
)

// Result separates infrastructure trust from the public TXT experiment.
type Result struct {
	Status      string `json:"status"`
	TrustSource string `json:"trust_source"`
	Detail      string `json:"detail"`
	Zone        string `json:"zone"`
}

// ParseAnchor accepts one public algorithm 13 DNSKEY or SHA-256 DS record.
func ParseAnchor(text, zone string) (dns.RR, error) {
	if len(text) > 4096 {
		return nil, errors.New("public anchor exceeds 4096 bytes")
	}
	text = strings.TrimSpace(text)
	if strings.ContainsAny(text, "\n\r$;()") {
		return nil, errors.New("require one public anchor record on one line")
	}
	rr, err := dns.New(text)
	if err != nil || rr == nil || rr.Header().Name != zone || rr.Header().Class != dns.ClassINET {
		return nil, errors.New("invalid zone-matched public anchor")
	}
	switch key := rr.(type) {
	case *dns.DNSKEY:
		if key.Algorithm == dns.ECDSAP256SHA256 && key.Protocol == 3 && key.Flags&dns.FlagZONE != 0 {
			return rr, nil
		}
	case *dns.DS:
		if key.Algorithm == dns.ECDSAP256SHA256 && key.DigestType == dns.SHA256 && len(key.Digest) == 64 {
			return rr, nil
		}
	}
	return nil, errors.New("require an algorithm 13 DNSKEY or SHA-256 DS public anchor")
}

// Verify checks the explicit anchor without private keys or local state access.
func Verify(ctx context.Context, address, zone string, anchor dns.RR, now time.Time) Result {
	result := Result{Status: "indeterminate", TrustSource: "none", Zone: zone, Detail: "complete public delegation verification is unsupported; supply an explicit public anchor"}
	if anchor == nil {
		return result
	}
	result.TrustSource = "explicit-local-anchor"
	if _, err := ParseAnchor(anchor.String(), zone); err != nil {
		result.Detail = "invalid public anchor"
		return result
	}
	if !validTarget(address, zone) || now.Unix() < 0 || now.Unix() > math.MaxUint32 {
		result.Detail = "invalid authority target or unsupported clock"
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	records, status := lookup(ctx, address, zone, dns.TypeDNSKEY)
	if status != "" {
		result.Status, result.Detail = status, "DNSKEY lookup fails"
		return result
	}
	keys := keysFrom(records)
	if len(keys) == 0 {
		result.Status, result.Detail = "insecure", "authority supplies no DNSKEY; delegation remains unverified"
		return result
	}
	trusted := anchored(keys, anchor)
	if !verifySet(records, dns.TypeDNSKEY, zone, trusted, now) {
		result.Status, result.Detail = "bogus", "DNSKEY set fails the explicit anchor or signature check"
		return result
	}
	records, status = lookup(ctx, address, zone, dns.TypeSOA)
	if status != "" {
		result.Status, result.Detail = status, "SOA lookup fails"
		return result
	}
	if !verifySet(records, dns.TypeSOA, zone, keys, now) {
		result.Status, result.Detail = "bogus", "SOA signature fails the authenticated DNSKEY set"
		return result
	}
	result.Status, result.Detail = "verified", "explicit anchor verifies current DNSKEY and SOA infrastructure; public delegation and experimental TXT validity remain separate"
	return result
}

func validTarget(address, zone string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || port == "" || len(zone) > 220 || zone != strings.ToLower(zone) || !strings.HasSuffix(zone, ".") {
		return false
	}
	labels := strings.Split(strings.TrimSuffix(zone, "."), ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !validLabel(label) {
			return false
		}
	}
	return true
}

func validLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, c := range label {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

func lookup(ctx context.Context, address, zone string, typ uint16) ([]dns.RR, string) {
	q := dns.NewMsg(zone, typ)
	q.Security, q.CheckingDisabled, q.UDPSize = true, true, 1232
	r, _, err := authority.Query(ctx, address, q)
	if err != nil {
		return nil, "unreachable"
	}
	if r == nil || r.Truncated || !r.Authoritative || r.Rcode != dns.RcodeSuccess || len(r.Answer) > 16 {
		return nil, "indeterminate"
	}
	return r.Answer, ""
}

func keysFrom(records []dns.RR) []*dns.DNSKEY {
	var keys []*dns.DNSKEY
	for _, rr := range records {
		if key, ok := rr.(*dns.DNSKEY); ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func anchored(keys []*dns.DNSKEY, anchor dns.RR) []*dns.DNSKEY {
	var matched []*dns.DNSKEY
	for _, key := range keys {
		switch a := anchor.(type) {
		case *dns.DNSKEY:
			if key.Flags == a.Flags && key.Protocol == a.Protocol && key.Algorithm == a.Algorithm && key.PublicKey == a.PublicKey {
				matched = append(matched, key)
			}
		case *dns.DS:
			ds := key.ToDS(dns.SHA256)
			if ds != nil && ds.KeyTag == a.KeyTag && ds.Algorithm == a.Algorithm && strings.EqualFold(ds.Digest, a.Digest) {
				matched = append(matched, key)
			}
		}
	}
	return matched
}

func verifySet(records []dns.RR, typ uint16, zone string, keys []*dns.DNSKEY, now time.Time) bool {
	var set []dns.RR
	var signatures []*dns.RRSIG
	for _, rr := range records {
		if rr.Header().Name != zone || rr.Header().Class != dns.ClassINET {
			return false
		}
		if dns.RRToType(rr) == typ {
			set = append(set, rr)
		} else if sig, ok := rr.(*dns.RRSIG); ok {
			signatures = append(signatures, sig)
		} else {
			return false
		}
	}
	if len(set) == 0 || len(records) > 16 {
		return false
	}
	for _, sig := range signatures {
		if !validSignature(sig, typ, zone, now) {
			continue
		}
		for _, key := range keys {
			if sig.Clone().(*dns.RRSIG).Verify(key, set, &dns.SignOption{}) == nil {
				return true
			}
		}
	}
	return false
}

func validSignature(sig *dns.RRSIG, typ uint16, zone string, now time.Time) bool {
	start, end, at := int64(sig.Inception), int64(sig.Expiration), now.Unix()
	return sig.TypeCovered == typ && sig.SignerName == zone && int(sig.Labels) == len(strings.Split(strings.TrimSuffix(zone, "."), ".")) &&
		end > start && end-start < 1<<31 && at >= start && at <= end
}
