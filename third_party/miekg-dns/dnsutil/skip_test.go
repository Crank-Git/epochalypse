package dnsutil

import (
	"fmt"
	"testing"
)

func TestSkip(t *testing.T) {
	testcases := []struct {
		in  string
		n   int
		dir skip
		out string
	}{
		{"www.example.org.", 3, SkipForward, ""},
		{"www.example.org.", 2, SkipForward, "org."},
		{"www.example.org.", 1, SkipForward, "example.org."},
		{"www.example.org.", 0, SkipForward, "www.example.org."},

		{"www.example.org.", 3, SkipBackward, "www.example.org."},
		{"www.example.org.", 2, SkipBackward, "example.org."},
		{"www.example.org.", 1, SkipBackward, "org."},
		{"www.example.org.", 0, SkipBackward, ""},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("%s%s%d", tc.in,
			func() string {
				if tc.dir == SkipForward {
					return "+"
				}
				return "-"
			}(),
			tc.n),
			func(t *testing.T) {
				x, _ := Skip(tc.in, tc.n, tc.dir)
				if x != tc.out {
					t.Fatalf("expected %s, got %s", tc.out, x)
				}
			})
	}
}
