package dnsnet

import (
	"fmt"
	"testing"
)

func TestFollowed(t *testing.T) {
	testcases := []struct {
		size    int
		follows int
	}{
		{5, minFollows},
		{2000, 2000 * octetFollows},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("size %d", tc.size), func(t *testing.T) {
			s := New(make([]byte, tc.size))
			for i := range tc.follows {
				if !s.Follow() {
					t.Fatalf("follow %d refused", i+1)
				}
			}
			if s.Follow() {
				t.Fatalf("follow %d allowed", tc.follows+1)
			}
		})
	}
}
