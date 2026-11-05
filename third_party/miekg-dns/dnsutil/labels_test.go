package dnsutil

import (
	"slices"
	"testing"
)

func TestJoin(t *testing.T) {
	testcases := []struct {
		in  []string
		out string
	}{
		{[]string{"bla", "bliep", "example", "org"}, "bla.bliep.example.org."},
		{[]string{"example", "."}, "example."},
		{[]string{"example", "org."}, "example.org."}, // technically we should not be called like this.
		{[]string{"."}, "."},
	}

	for i, tc := range testcases {
		if x := Join(tc.in...); x != tc.out {
			t.Errorf("test %d, expected %s, got %s", i, tc.out, x)
		}
	}
}

func TestSplit(t *testing.T) {
	testcases := []struct {
		in  string
		out []string
	}{
		{"bla.bliep.example.org.", []string{"bla", "bliep", "example", "org"}},
		{"example.org", []string{"example", "org"}},
		{".", []string{"."}},
		{"", []string{""}},
	}

	for i, tc := range testcases {
		if x := Split(tc.in); slices.Compare(x, tc.out) != 0 {
			t.Errorf("test %d, expected %v, got %v", i, tc.out, x)
		}
	}
}

func TestNext(t *testing.T) {
	type next struct {
		string
		int
	}
	testcases := map[next]int{
		{"", 1}:             0,
		{"www.miek.nl.", 0}: 4,
		{"www.miek.nl.", 4}: 9,
		{"www.miek.nl.", 9}: 12,
	}
	for tc, i := range testcases {
		x, ok := Next(tc.string, tc.int)
		if i != x {
			t.Errorf("label should be %d, got %d, %t: next %d, %s", i, x, ok, tc.int, tc.string)
		}
	}
}

func TestForward(t *testing.T) {
	testcases := []struct {
		in  string
		out []string
	}{
		{".", []string{"."}},
		{"nl.", []string{"nl"}},
		{"www.miek.nl.", []string{"www", "miek", "nl"}},
	}
	for _, tc := range testcases {
		t.Run(tc.in, func(t *testing.T) {
			out := []string{}
			for x := range Forward(tc.in) {
				out = append(out, x)
			}
			if slices.Compare(tc.out, out) != 0 {
				t.Fatalf("labels should be %v, got %v", tc.out, out)
			}
		})
	}
}

func TestPrev(t *testing.T) {
	type prev struct {
		string
		int
	}
	testcases := map[prev]int{
		{"", 1}:              0,
		{".", 1}:             0,
		{"www.miek.nl.", 12}: 9,
		{"www.miek.nl.", 9}:  4,
		{"www.miek.nl.", 4}:  0,
	}
	for tc, i := range testcases {
		x, ok := Prev(tc.string, tc.int)
		if i != x {
			t.Errorf("label should be %d, got %d, %t: prev2 %d, %s", i, x, ok, tc.int, tc.string)
		}
	}
}

func TestBackward(t *testing.T) {
	testcases := []struct {
		in  string
		out []string
	}{
		{".", []string{"."}},
		{"nl.", []string{"nl"}},
		{"www.miek.nl.", []string{"nl", "miek", "www"}},
	}
	for _, tc := range testcases {
		t.Run(tc.in, func(t *testing.T) {
			out := []string{}
			for x := range Backward(tc.in) {
				out = append(out, x)
			}
			if slices.Compare(tc.out, out) != 0 {
				t.Fatalf("labels should be %v, got %v", tc.out, out)
			}
		})
	}
}

func TestLabels(t *testing.T) {
	testcases := map[string]int{
		"www.miek.nl.": 3,
		"www.miek.nl":  3,
		"nl":           1,
		".":            0,
	}
	for tc, i := range testcases {
		x := Labels(tc)
		if x != i {
			t.Errorf("labels should have %d, got %d", i, x)
		}
	}
}

func TestCanonical(t *testing.T) {
	for s, expect := range map[string]string{
		"":                 ".",
		".":                ".",
		"tld":              "tld.",
		"tld.":             "tld.",
		"example.test":     "example.test.",
		"Lower.CASE.test.": "lower.case.test.",
		"*.Test":           "*.test.",
		"ÉxamplE.com":      "Éxample.com.",
		"É.com":            "É.com.",
	} {
		if got := Canonical(s); got != expect {
			t.Errorf("Canonical(%q) = %q, expected %q", s, got, expect)
		}
	}
}

func TestIsName(t *testing.T) {
	testcases := map[string]bool{
		".":                 true,
		"..":                false,
		"double-dot..test":  false,
		".leading-dot.test": false,
		"@.":                true,
		"www.example.com":   true,
		"www.e%ample.com":   true,
		"www.example.com.":  true,
		"mi\\k.nl.":         true,
		"mi\\k.nl":          true,
		"":                  true,
	}
	for tc, ok := range testcases {
		ok1 := IsName(tc)
		if ok != ok1 {
			t.Errorf("have %t for %s ", ok, tc)
		}
	}
}
