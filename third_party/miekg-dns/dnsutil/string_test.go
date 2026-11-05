package dnsutil

import (
	"fmt"
	"testing"

	"codeberg.org/miekg/dns"
)

func TestToString(t *testing.T) {
	if x := RcodeToString(5); x != "REFUSED" {
		t.Errorf("expected %s, got %s", "REFUSED", x)
	}
	if x := RcodeToString(55); x != "RCODE55" {
		t.Errorf("expected %s, got %s", "RCODE55", x)
	}
	if x := OpcodeToString(0); x != "QUERY" {
		t.Errorf("expected %s, got %s", "QUERY", x)
	}
	if x := OpcodeToString(12); x != "OPCODE12" {
		t.Errorf("expected %s, got %s", "OPCODE12", x)
	}
	if x := TypeToString(1); x != "A" {
		t.Errorf("expected %s, got %s", "A", x)
	}
	if x := ClassToString(1); x != "IN" {
		t.Errorf("expected %s, got %s", "IN", x)
	}
	if x := CodeToString(10); x != "COOKIE" {
		t.Errorf("expected %s, got %s", "COOKIE", x)
	}
}

func TestStringToType(t *testing.T) {
	testcases := map[string]struct {
		Number uint16
		Fn     func(s string) (uint16, error)
		Err    bool
	}{
		// Type
		"A":                      {dns.TypeA, StringToType, true},
		"AAAA":                   {dns.TypeAAAA, StringToType, true},
		"a":                      {dns.TypeA, StringToType, true},
		"banana":                 {0, StringToType, false},
		"type1":                  {dns.TypeA, StringToType, true},
		"typex":                  {0, StringToType, false},
		"type100000":             {0, StringToType, false},
		TypeToString(dns.TypeMX): {dns.TypeMX, StringToType, true},
		TypeToString(60000):      {60000, StringToType, true},
		// Code
		"REFUSED":                         {dns.RcodeRefused, StringToRcode, true},
		"refused":                         {dns.RcodeRefused, StringToRcode, true},
		"orange":                          {0, StringToRcode, false},
		"rcode5":                          {dns.RcodeRefused, StringToRcode, true},
		"RCODE55":                         {55, StringToRcode, true},
		"rcodex":                          {0, StringToRcode, false},
		"rcode100000":                     {0, StringToRcode, false},
		RcodeToString(dns.RcodeBadCookie): {dns.RcodeBadCookie, StringToRcode, true},
		RcodeToString(55):                 {55, StringToRcode, true},
		// Class
		"IN":                          {dns.ClassINET, StringToClass, true},
		"in":                          {dns.ClassINET, StringToClass, true},
		"appel":                       {0, StringToClass, false},
		"CLass1":                      {dns.ClassINET, StringToClass, true},
		"classx":                      {0, StringToClass, false},
		"class100000":                 {0, StringToClass, false},
		ClassToString(dns.ClassCHAOS): {dns.ClassCHAOS, StringToClass, true},
		ClassToString(200):            {200, StringToClass, true},
		// Code
		"COOKIE":                       {dns.CodeCOOKIE, StringToCode, true},
		"cookie":                       {dns.CodeCOOKIE, StringToCode, true},
		"pear":                         {0, StringToCode, false},
		"code10":                       {dns.CodeCOOKIE, StringToCode, true},
		"codex":                        {0, StringToCode, false},
		"code100000":                   {0, StringToCode, false},
		CodeToString(dns.CodeLOCALEND): {dns.CodeLOCALEND, StringToCode, true},
		CodeToString(200):              {200, StringToCode, true},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got, err := tc.Fn(name)
			if !tc.Err && err == nil {
				t.Errorf("expected error, got nil")
			}
			if tc.Err {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if got != tc.Number {
					t.Fatalf("expected %v, got %v", tc.Number, got)
				}
			}
		})
	}
}

func TestStringToOpcode(t *testing.T) {
	testcases := map[string]struct {
		Opcode uint8
		Err    bool
	}{
		"QUERY":                            {dns.OpcodeQuery, true},
		"status":                           {dns.OpcodeStatus, true},
		"banana":                           {0, false},
		"opcode0":                          {dns.OpcodeQuery, true},
		"opcodex":                          {0, false},
		"opcode300":                        {0, false},
		OpcodeToString(dns.OpcodeStateful): {dns.OpcodeStateful, true},
		OpcodeToString(20):                 {20, true},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got, err := StringToOpcode(name)
			if !tc.Err && err == nil {
				t.Errorf("expected error, got nil")
			}
			if tc.Err {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if got != tc.Opcode {
					t.Errorf("expected %v, got %v", tc.Opcode, got)
				}
			}
		})
	}
}

func ExampleTypeToString() {
	rr := &dns.MX{Hdr: dns.Header{Name: "miek.nl.", Class: dns.ClassINET, TTL: 3600}, Preference: 10, Mx: "mx.miek.nl."}
	fmt.Println(TypeToString(dns.RRToType(rr)))
	// Output: MX
}
