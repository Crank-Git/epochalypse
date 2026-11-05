package dnsutil

import (
	"testing"
)

func TestNSEC3Name(t *testing.T) {
	testcases := []struct {
		in, exp string
	}{
		// Cases from RFC 5155, Appendix A.
		{"example.", "0P9MHAVEQVM6T7VBL5LOP2U3T2RP3TOM"},
		{"ns2.example.", "Q04JKCEVQVMU85R014C7DKBA38O0JI5R"},
		{"*.w.example.", "R53BQ7CC2UVMUBFU5OCMM6PERS9TK9EN"},

		{"ai.example.", "GJEQE526PLBF1G8MKLP59ENFD789NJGI"},
		{"AI.example.", "GJEQE526PLBF1G8MKLP59ENFD789NJGI"},
		{"Ns2.EXAMPLE.", "Q04JKCEVQVMU85R014C7DKBA38O0JI5R"},
	}

	for i, tc := range testcases {
		got := NSEC3Name(tc.in, "aabbccdd", 12)
		if got != tc.exp {
			t.Errorf("test %d, expected %s, got %s", i, tc.exp, got)
		}
	}
}
