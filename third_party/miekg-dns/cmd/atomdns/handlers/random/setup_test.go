package random

import (
	"testing"

	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnsserver"
)

func TestSetup(t *testing.T) {
	testcases := []struct {
		input string
		fail  bool
	}{
		{`random`, false},
		{`random random`, false},
		{`random bla`, true},
	}
	for i, tc := range testcases {
		random := new(Random)
		co := dnsserver.NewTestController(tc.input)
		err := random.Setup(co)

		if tc.fail && err == nil {
			t.Errorf("test %d: expected error, got nothing", i)
		}
		if !tc.fail && err != nil {
			t.Errorf("test %d: expected no error, got %s", i, err)
		}
	}
}
