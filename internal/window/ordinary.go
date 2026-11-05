package window

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"codeberg.org/miekg/dns"
)

// SignCurrentTXT signs a public TXT record with ordinary validity under the configured key.
// Each call renews validity independently of any event in the record.
func (s *Signer) SignCurrentTXT(txt *dns.TXT, now time.Time) (*dns.RRSIG, *dns.DNSKEY, error) {
	seconds := now.Unix()
	if txt == nil || seconds < 300 || seconds > 1<<32-1-48*3600 {
		return nil, nil, errors.New("ordinary TXT signature requires a supported clock and record")
	}
	if txt.Hdr.Class != dns.ClassINET || !strings.HasSuffix(txt.Hdr.Name, "."+s.key.Hdr.Name) || !ordinaryName(txt.Hdr.Name) {
		return nil, nil, errors.New("ordinary TXT record must use a canonical name below the configured zone")
	}
	// A five-minute allowance tolerates clock differences without coupling validity to an event.
	sig := dns.NewRRSIG(s.key.Hdr.Name, s.key.Algorithm, s.keyTag, uint32(seconds-300), uint32(seconds+48*3600)) // #nosec G115 -- The clock check bounds both timestamps.
	if err := sig.Sign(s.private, []dns.RR{txt}, &dns.SignOption{}); err != nil {
		return nil, nil, fmt.Errorf("sign ordinary public TXT: %w", err)
	}
	key := *s.key
	return sig, &key, nil
}

func ordinaryName(name string) bool {
	if len(name) > 254 || name != strings.ToLower(name) || !strings.HasSuffix(name, ".") {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if !validLabel(label) {
			return false
		}
	}
	return true
}
