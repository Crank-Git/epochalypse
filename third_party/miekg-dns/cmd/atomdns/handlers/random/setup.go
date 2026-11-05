package random

import (
	"fmt"

	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnsserver"
)

func (r *Random) Setup(co *dnsserver.Controller) error {
	for co.Next() {
		args := co.RemainingArgs()
		if len(args) == 0 {
			return nil
		}
		if len(args) > 1 {
			return co.ArgErr()
		}
		if args[0] != "random" {
			return co.PropErr(fmt.Errorf("policy can only be '%s'", "random"))
		}
	}
	return nil
}
