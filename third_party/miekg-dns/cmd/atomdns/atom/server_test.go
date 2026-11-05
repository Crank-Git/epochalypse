package atom_test

import (
	"fmt"
	"testing"

	"codeberg.org/miekg/dns/cmd/atomdns/atomtest"
)

func TestServer(t *testing.T) {
	testcases := []struct {
		name   string
		global string
		config string
		err    error
	}{
		{
			"origin-registered",
			"",
			`
example.org {
	log
}
example.org {
	log
}
		`,
			fmt.Errorf("origin already registered"),
		},
		{
			"duplicate-origin-but-classes",
			"",
			`
{
	dns {
		addr [::]:0
	}
}

example.org/IN {
	log
}
example.org/CH {
	log
}
		`,
			nil,
		},
		{
			"tls-required",
			`
{
	dns {
		addr [::]:0
	}
	doh {
		addr [::]:0
	}
}
`,
			`
example.org {
	log
}
`,
			fmt.Errorf("doh requires tls"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				err    error
				cancel func()
			)
			if tc.global == "" {
				_, cancel, err = atomtest.New(tc.config)
			}
			_, cancel, err = atomtest.New(tc.config, tc.global)
			if cancel != nil {
				defer cancel()
			}

			if tc.err == nil && err != nil {
				t.Fatalf("expected not error, but got: %s", err)
			}
			if tc.err != nil && err == nil {
				t.Fatalf("expected error %s, got none", tc.err)
			}
		})
	}
}
