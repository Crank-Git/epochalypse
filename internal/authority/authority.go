// Package authority serves signed window and countdown zones without recursive service.
package authority

import (
	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
	"errors"
	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/window"
	"strings"
	"sync"
	"time"
)

// Authority reads committed records and serves one signed zone.
type Authority struct {
	store            *window.Store
	signer           *window.Signer
	key              infrastructureSigner
	countdowns       *countdown.Store
	countdownSigner  *countdown.Signer
	zone, nameserver string
	mu               sync.Mutex
	cache            *snapshot
}

type infrastructureSigner interface {
	PublicKey() *dns.DNSKEY
	SignInfrastructure([]dns.RR, time.Time) (*dns.RRSIG, error)
}

// NewCountdown prepares the separate countdown zone with ordinary DNSSEC infrastructure.
func NewCountdown(store *countdown.Store, signer *countdown.Signer, nameserver string) (*Authority, error) {
	if store == nil || signer == nil {
		return nil, errors.New("configure the countdown store and signer")
	}
	zone := signer.PublicKey().Hdr.Name
	if err := ValidateNameServer(zone, nameserver); err != nil {
		return nil, err
	}
	return &Authority{countdowns: store, countdownSigner: signer, key: signer, zone: zone, nameserver: nameserver}, nil
}

// New prepares an authority with an explicit nameserver outside the experiment.
func New(store *window.Store, signer *window.Signer, nameserver string) (*Authority, error) {
	if store == nil || signer == nil {
		return nil, errors.New("configure the signed window store and signer")
	}
	zone := signer.PublicKey().Hdr.Name
	if err := ValidateNameServer(zone, nameserver); err != nil {
		return nil, err
	}
	return &Authority{store: store, signer: signer, key: signer, zone: zone, nameserver: nameserver}, nil
}

// ValidateNameServer requires an explicit canonical hostname outside the experimental zone.
func ValidateNameServer(zone, name string) error {
	if !dnsutil.IsName(name) || dnsutil.Canonical(name) != name || name == "." || inZone(name, zone) {
		return errors.New("configure a canonical nameserver outside the experimental zone")
	}
	return nil
}

// Packet returns a bounded wire response after it unpacks the complete request.
func (a *Authority) Packet(data []byte, udp bool, now time.Time) []byte {
	if len(data) < dns.MsgHeaderSize {
		return nil
	}
	if data[2]&0x80 != 0 {
		return nil
	}
	if data[4] != 0 || data[5] != 1 {
		return headerError(data, dns.RcodeRefused)
	}
	if rcode := wireNamePolicy(data); rcode != dns.RcodeSuccess {
		return headerError(data, rcode)
	}
	request := &dns.Msg{Data: data}
	if err := request.Unpack(); err != nil {
		return headerError(data, dns.RcodeFormatError)
	}
	reply := dnsutil.SetReply(new(dns.Msg), request)
	if !a.accepts(request) {
		reply.Rcode = dns.RcodeRefused
		return wire(reply)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	snap, err := a.current(now)
	if err != nil {
		reply.Rcode = dns.RcodeServerFailure
		return wire(reply)
	}
	reply.Authoritative = true
	reply.UDPSize = min(max(request.UDPSize, 512), 1232)
	snap.answer(reply, dnsutil.Canonical(request.Question[0].Header().Name), dns.RRToType(request.Question[0]), request.Security)
	if udp && reply.Len() > int(reply.UDPSize) {
		dnsutil.Truncate(reply)
	}
	return wire(reply)
}

func (a *Authority) accepts(r *dns.Msg) bool {
	if r.Opcode != dns.OpcodeQuery || len(r.Question) != 1 {
		return false
	}
	q := r.Question[0]
	if q.Header().Class != dns.ClassINET || !inZone(dnsutil.Canonical(q.Header().Name), a.zone) {
		return false
	}
	switch dns.RRToType(q) {
	case dns.TypeANY, dns.TypeAXFR, dns.TypeIXFR:
		return false
	}
	return true
}

func inZone(name, zone string) bool { return name == zone || strings.HasSuffix(name, "."+zone) }

func headerError(data []byte, rcode uint16) []byte {
	r := new(dns.Msg)
	r.ID = uint16(data[0])<<8 | uint16(data[1])
	r.Response = true
	r.Rcode = rcode
	return wire(r)
}

func wire(m *dns.Msg) []byte {
	if err := m.Pack(); err != nil {
		return nil
	}
	return m.Data
}

// Metadata contains public records for a local anchor or parent publication.
type Metadata struct {
	Zone       string
	NameServer string
	DNSKEY     *dns.DNSKEY
	ParentDS   *dns.DS
}

// Metadata returns public key material for an anchor or the delegating parent.
func (a *Authority) Metadata() Metadata {
	key := a.key.PublicKey()
	return Metadata{Zone: a.zone, NameServer: a.nameserver, DNSKEY: key, ParentDS: key.ToDS(dns.SHA256)}
}
