package panic

import (
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnsserver"
)

func (p *Panic) Setup(co *dnsserver.Controller) error {
	co.OnStartup(func() error {
		log().Info("Startup", "panic", "recover")
		return nil
	})
	return nil
}
