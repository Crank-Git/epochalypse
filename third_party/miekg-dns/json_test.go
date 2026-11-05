package dns

import (
	"testing"
)

func TestMarshalJSON(t *testing.T) {
	testcases := []struct {
		name string
		rrs  []RR
	}{
		{
			name: "single",
			rrs: []RR{
				dnstestNew("www.example.org. IN A 127.0.0.1"),
			},
		},
		{
			name: "multiple",
			rrs: []RR{
				dnstestNew("www.example.org. IN A 127.0.0.1"),
				dnstestNew("www.example.org. IN A 127.0.0.2"),
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			jsonb, err := MarshalJSON(tc.rrs...)
			if err != nil {
				t.Fatal(err)
			}
			rrs, err := UnmarshalJSON(jsonb)
			if err != nil {
				t.Fatal(err)
			}
			for i, rr := range tc.rrs {
				if !Equal(rr, rrs[i]) {
					t.Fatalf("expected %s and %s to be equal", rr, rrs[i])
				}
			}
		})
	}
}
