package authority

import (
	"codeberg.org/miekg/dns"
	"slices"
	"strings"
	"time"
)

type snapshot struct {
	revision int
	hour     int64
	zone     string
	names    []string
	nodes    map[string]map[uint16][]dns.RR
}

func (a *Authority) current(now time.Time) (*snapshot, error) {
	revision, err := a.store.Revision()
	if err != nil {
		return nil, err
	}
	if a.cache != nil && a.cache.revision == revision && a.cache.hour == now.Unix()/3600 {
		return a.cache, nil
	}
	windows, revision, err := a.store.Snapshot()
	if err != nil {
		return nil, err
	}
	s := &snapshot{revision: revision, hour: now.Unix() / 3600, zone: a.zone, names: []string{a.zone}, nodes: map[string]map[uint16][]dns.RR{a.zone: {}}}
	for _, w := range windows {
		txt, sig, _, err := w.Records()
		if err != nil {
			return nil, err
		}
		s.names = append(s.names, w.Name)
		s.nodes[w.Name] = map[uint16][]dns.RR{dns.TypeTXT: {txt, sig}}
	}
	slices.SortFunc(s.names, compareName)
	if err := a.infrastructure(s, now); err != nil {
		return nil, err
	}
	a.cache = s
	return s, nil
}

func (a *Authority) infrastructure(s *snapshot, now time.Time) error {
	header := dns.Header{Name: a.zone, TTL: 60, Class: dns.ClassINET}
	soa := &dns.SOA{Hdr: header, Ns: a.nameserver, Mbox: "hostmaster." + a.zone, Serial: uint32(s.revision), Refresh: 60, Retry: 60, Expire: 86400, Minttl: 60} // #nosec G115 -- SOA serials deliberately wrap modulo 2^32.
	for _, rr := range []dns.RR{a.signer.PublicKey(), &dns.NS{Hdr: header, Ns: a.nameserver}, soa} {
		if err := a.addInfrastructure(s, rr, now); err != nil {
			return err
		}
	}
	for i, name := range s.names {
		types := []uint16{dns.TypeRRSIG, dns.TypeNSEC}
		for typ := range s.nodes[name] {
			types = append(types, typ)
		}
		slices.Sort(types)
		nsec := &dns.NSEC{Hdr: dns.Header{Name: name, TTL: 60, Class: dns.ClassINET}, NextDomain: s.names[(i+1)%len(s.names)], TypeBitMap: types}
		if err := a.addInfrastructure(s, nsec, now); err != nil {
			return err
		}
	}
	return nil
}

func (a *Authority) addInfrastructure(s *snapshot, rr dns.RR, now time.Time) error {
	sig, err := a.signer.SignInfrastructure([]dns.RR{rr}, now)
	if err != nil {
		return err
	}
	s.nodes[rr.Header().Name][dns.RRToType(rr)] = []dns.RR{rr, sig}
	return nil
}

func (s *snapshot) answer(reply *dns.Msg, name string, typ uint16, secure bool) {
	node, exists := s.nodes[name]
	if exists {
		if typ == dns.TypeRRSIG {
			reply.Answer = signatures(node)
			return
		}
		if records, ok := node[typ]; ok {
			reply.Answer = recordsFor(records, secure)
			return
		}
	} else {
		reply.Rcode = dns.RcodeNameError
	}
	reply.Ns = recordsFor(s.nodes[s.zone][dns.TypeSOA], secure)
	if !secure {
		return
	}
	index := s.predecessor(name)
	reply.Ns = append(reply.Ns, s.nodes[s.names[index]][dns.TypeNSEC]...)
	if exists {
		return
	}
	wildcard := s.predecessor("*." + s.closest(name))
	if wildcard != index {
		reply.Ns = append(reply.Ns, s.nodes[s.names[wildcard]][dns.TypeNSEC]...)
	}
}

func recordsFor(records []dns.RR, secure bool) []dns.RR {
	if secure {
		return records
	}
	return records[:1]
}

func signatures(node map[uint16][]dns.RR) []dns.RR {
	types := make([]uint16, 0, len(node))
	for typ := range node {
		types = append(types, typ)
	}
	slices.Sort(types)
	records := make([]dns.RR, 0, len(types))
	for _, typ := range types {
		records = append(records, node[typ][1])
	}
	return records
}

func (s *snapshot) predecessor(name string) int {
	index, found := slices.BinarySearchFunc(s.names, name, compareName)
	if !found {
		index--
	}
	if index < 0 {
		index = len(s.names) - 1
	}
	return index
}

func (s *snapshot) closest(name string) string {
	for name != "" {
		if _, exists := s.nodes[name]; exists {
			return name
		}
		_, name, _ = strings.Cut(name, ".")
	}
	return s.zone
}

func compareName(a, b string) int {
	left, right := strings.Split(strings.TrimSuffix(a, "."), "."), strings.Split(strings.TrimSuffix(b, "."), ".")
	for i, j := len(left)-1, len(right)-1; i >= 0 && j >= 0; i, j = i-1, j-1 {
		if order := strings.Compare(left[i], right[j]); order != 0 {
			return order
		}
	}
	return len(left) - len(right)
}
