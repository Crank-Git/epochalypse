package panic

import (
	"codeberg.org/miekg/dns/cmd/atomdns/handlers/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const subsystem = "panic"

var Panics = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: metrics.Namespace, Subsystem: subsystem,
	Name: "panics_total",
	Help: "Counter of recovered panics.",
})
