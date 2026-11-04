package window

import (
	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// PublicKey returns an independent public key without private material.
func (s *Signer) PublicKey() *dns.DNSKEY { return s.key.Clone().(*dns.DNSKEY) }

// SignInfrastructure signs current infrastructure without an alternate TXT signature.
func (s *Signer) SignInfrastructure(records []dns.RR, now time.Time) (*dns.RRSIG, error) {
	if !dnsutil.IsRRset(records) {
		return nil, errors.New("require one infrastructure RRset")
	}
	if err := s.infrastructureType(records[0]); err != nil {
		return nil, err
	}
	seconds := now.Unix()
	if seconds < 0 || seconds > math.MaxUint32 {
		return nil, errors.New("infrastructure clock exceeds the supported timestamp range")
	}
	hour := seconds / 3600 * 3600
	inception, expiration := max(int64(0), hour-300), min(int64(math.MaxUint32), hour+86400)
	sig := dns.NewRRSIG(s.key.Hdr.Name, s.key.Algorithm, s.key.KeyTag(), uint32(inception), uint32(expiration)) // #nosec G115 -- Both values are bounded to uint32 above.
	if err := sig.Sign(s.private, records, &dns.SignOption{}); err != nil {
		return nil, fmt.Errorf("sign infrastructure RRset: %w", err)
	}
	return sig, nil
}

func (s *Signer) infrastructureType(rr dns.RR) error {
	name := rr.Header().Name
	if rr.Header().Class != dns.ClassINET || (name != s.key.Hdr.Name && !strings.HasSuffix(name, "."+s.key.Hdr.Name)) {
		return errors.New("infrastructure RRset lies outside the configured zone")
	}
	switch dns.RRToType(rr) {
	case dns.TypeDNSKEY, dns.TypeNS, dns.TypeSOA:
		if name != s.key.Hdr.Name {
			return errors.New("infrastructure apex RRset has another owner name")
		}
	case dns.TypeNSEC:
	default:
		return errors.New("infrastructure signer rejects this RR type")
	}
	return nil
}
