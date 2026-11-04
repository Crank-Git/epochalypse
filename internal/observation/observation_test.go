package observation

import (
	"context"
	"net"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

func failureResponder(t *testing.T) string {
	t.Helper()
	connection, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { _ = connection.Close(); <-done })
	go func() {
		defer close(done)
		data := make([]byte, 1232)
		n, address, err := connection.ReadFrom(data)
		if err != nil {
			return
		}
		request := &dns.Msg{Data: data[:n]}
		if request.Unpack() != nil {
			return
		}
		response := new(dns.Msg)
		dnsutil.SetReply(response, request)
		response.Rcode = dns.RcodeServerFailure
		response.RecursionAvailable = true
		response.CheckingDisabled = false
		if response.Pack() != nil {
			return
		}
		_, _ = connection.WriteTo(response.Data, address)
	}()
	return connection.LocalAddr().String()
}

func TestObservePreservesResponseAndProvenance(t *testing.T) {
	offset := int64(0)
	target := Target{ID: "unbound-strict", Address: failureResponder(t), Version: "1.22.0", VersionSource: "configured", Clock: Clock{OffsetNanoseconds: &offset, Source: "configured"}}
	before := time.Now()
	result := Observe(context.Background(), "public.example.", target)
	if result.Rcode == nil || *result.Rcode != dns.RcodeServerFailure || result.Flags == nil || !result.Flags.RA || result.Flags.AD {
		t.Fatalf("the observation loses the response: %+v", result)
	}
	if result.Version != 1 || result.Resolver.Version != target.Version || result.Resolver.Clock.Source != "configured" || result.Transport != "udp" || result.Started.Before(before) || result.Completed.Before(result.Started) || result.Error != "" {
		t.Fatalf("the observation loses its environment: %+v", result)
	}
}

func TestObservePreservesUnavailableResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := Observe(ctx, "public.example.", Target{ID: "unavailable", Address: "127.0.0.1:1", Version: "unknown", VersionSource: "unknown", Clock: Clock{Source: "unknown"}})
	if result.Error != "resolver_unavailable" || result.Rcode != nil || result.Flags != nil || result.Completed.Before(result.Started) {
		t.Fatalf("the failed query claims a response: %+v", result)
	}
}
