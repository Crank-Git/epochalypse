// Package service provides local listeners and capsule release interfaces.
package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Crank-Git/epochalypse/internal/runtimeinfo"
)

// Addresses contains the bound listener addresses.
type Addresses struct{ HTTP, DNS string }

// Handler returns the HTTP routes for a local process.
func Handler() *http.ServeMux {
	mux := http.NewServeMux()
	runtimeinfo.Routes(mux, func() runtimeinfo.Report { return runtimeinfo.New("scaffold", "") })
	return mux
}

// Serve runs a process until the context ends or a listener fails.
// The ready callback receives the bound addresses after all listeners start.
func Serve(ctx context.Context, config Config, ready func(Addresses)) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(config.StateDir, "v1"), 0700); err != nil {
		return fmt.Errorf("create versioned state directory: %w", err)
	}
	var lc net.ListenConfig
	httpListener, err := lc.Listen(ctx, "tcp", config.HTTPAddr)
	if err != nil {
		return fmt.Errorf("bind HTTP address: %w", err)
	}
	defer func() { _ = httpListener.Close() }()
	addresses := Addresses{HTTP: httpListener.Addr().String()}
	results := make(chan error, 3)
	handler := Handler()
	var respond func([]byte, bool) []byte
	var state *mainState
	if config.Mode == "server" {
		state, err = openMainState(config)
		if err != nil {
			return err
		}
		defer state.close()
		handler = http.NewServeMux()
		runtimeinfo.Routes(handler, func() runtimeinfo.Report { return state.readiness(config) })
		state.routes(handler, config.LocalLab)
		respond = state.dnsResponder()
	}
	if config.Mode == "dns" || respond != nil {
		dns, err := openDNS(ctx, config.DNSAddr)
		if err != nil {
			return err
		}
		defer dns.close()
		dns.respond = respond
		addresses.DNS = dns.tcp.Addr().String()
		state.setDirectAddress(dns.tcp.Addr())
		go func() { results <- dns.serveUDP() }()
		go func() { results <- dns.serveTCP() }()
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	defer func() { _ = server.Close() }()
	go func() {
		if config.TLSCert != "" {
			results <- server.ServeTLS(httpListener, config.TLSCert, config.TLSKey)
			return
		}
		results <- server.Serve(httpListener)
	}()
	ready(addresses)
	select {
	case <-ctx.Done():
		return nil
	case err := <-results:
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return fmt.Errorf("serve local process: %w", err)
	}
}
