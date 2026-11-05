package countdown

import (
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/window"
)

// Signer uses the separate configured countdown key without event-boundary validity.
type Signer struct {
	zone string
	key  *window.Signer
}

// LoadSigner verifies an existing protected key reference for the countdown zone.
func LoadSigner(zone, path string) (*Signer, error) {
	if !ValidZone(zone) {
		return nil, ErrInput
	}
	key, err := window.LoadSigner(zone, path)
	if err != nil {
		return nil, err
	}
	return &Signer{zone: zone, key: key}, nil
}

// LabOnly reports the explicit lab marker for service admission checks.
func (s *Signer) LabOnly() bool { return s.key.LabOnly() }

// Records returns independent signed records after it validates the retained metadata.
// Each call renews ordinary validity, including after the event completes.
func (s *Signer) Records(r Record, now time.Time) (*dns.TXT, *dns.RRSIG, *dns.DNSKEY, error) {
	if err := r.validate(); err != nil {
		return nil, nil, nil, err
	}
	if r.Zone != s.zone {
		return nil, nil, nil, ErrIntegrity
	}
	txt := r.TXT()
	sig, key, err := s.key.SignCurrentTXT(txt, now)
	if err != nil {
		return nil, nil, nil, err
	}
	return txt, sig, key, nil
}
