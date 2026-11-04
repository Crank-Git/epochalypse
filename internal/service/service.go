// Package service provides local listeners without release behavior.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/version"
)

// Addresses contains the bound listener addresses.
type Addresses struct{ HTTP, DNS string }

// Handler returns the HTTP routes for a local process.
func Handler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Commit string `json:"commit"`
		}{version.Commit})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
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
	var dns *dnsListeners
	if config.Mode == "dns" {
		dns, err = openDNS(ctx, config.DNSAddr)
		if err != nil {
			return err
		}
		defer dns.close()
		addresses.DNS = dns.tcp.Addr().String()
		go func() { results <- dns.serveUDP() }()
		go func() { results <- dns.serveTCP() }()
	}
	handler := Handler()
	if config.Mode == "server" {
		store, err := owner.Open(config.StateDir)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		ownerHandler := store.Handler(config.LocalLab)
		for _, path := range []string{"/owner", "/owner/", "/api/v1/session"} {
			handler.Handle(path, ownerHandler)
		}
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
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
