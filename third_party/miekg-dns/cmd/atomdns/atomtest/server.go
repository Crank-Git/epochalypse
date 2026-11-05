package atomtest

import (
	"context"
	"strings"

	"codeberg.org/miekg/dns/cmd/atomdns/atom"
)

// New returns a server suitable for testing. Use cancel to shutdown the server
// Use [server.Addr] to get the listening addresses. NewTest starts 2 servers, one on UDP and another on TCP.
// The config will be prefixed with, unless global is set in which case that string is used as the global
// block.
//
//	{
//		dns {
//			addr [::]:0
//			limits {
//				run 1
//			}
//		}
//	}
func New(config string, global ...string) (server *atom.Server, cancel func(), err error) {
	testconfig := `
{
	dns {
		addr [::]:0
		limits {
			run 1
		}
	}
}
`
	if len(global) > 0 {
		testconfig = global[0]
	}

	config = testconfig + config
	s, err := atom.New("<test>", strings.NewReader(config))
	if err != nil {
		return nil, nil, err
	}
	if err := s.Start(); err != nil {
		return nil, nil, err
	}
	return s, func() { s.Shutdown(context.TODO()) }, nil
}
