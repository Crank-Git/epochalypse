package dnsutil

import "codeberg.org/miekg/dns"

func ExampleSetQuestion_unknownType() {
	m := new(dns.Msg)
	m.ID = dns.ID()
	m.RecursionDesired = true
	m.Question[0] = &dns.RFC3597{
		Hdr:    dns.Header{Name: "www.example.org.", Class: dns.ClassINET},
		RRType: 1000,
	}
}
