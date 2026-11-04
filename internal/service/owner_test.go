package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServerRequiresTLSForOwnerAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan Addresses, 1)
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, Config{Mode: "server", HTTPAddr: "127.0.0.1:0", StateDir: t.TempDir()}, func(a Addresses) { ready <- a })
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	select {
	case address := <-ready:
		client := &http.Client{Timeout: time.Second}
		response, err := client.Get("http://" + address.HTTP + "/owner")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != 426 {
			t.Fatalf("owner access: %d", response.StatusCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server start timeout")
	}
}

func TestOwnerConfigRejectsUnsafeLabAndIncompleteTLS(t *testing.T) {
	for _, settings := range []Config{
		{HTTPAddr: "0.0.0.0:8080", LocalLab: true},
		{HTTPAddr: "localhost:8080", LocalLab: true},
		{HTTPAddr: "bad", LocalLab: true},
		{HTTPAddr: "127.0.0.1:0", TLSCert: "cert"},
		{HTTPAddr: "127.0.0.1:0", TLSKey: "key"},
	} {
		settings.Mode = "server"
		settings.StateDir = t.TempDir()
		if err := settings.Validate(); err == nil {
			t.Fatalf("unsafe configuration accepted: %+v", settings)
		}
	}
}

func TestServerServesOwnerAccessOverTLS(t *testing.T) {
	fixture := httptest.NewTLSServer(http.NewServeMux())
	defer fixture.Close()
	directory := t.TempDir()
	cert := filepath.Join(directory, "cert.pem")
	key := filepath.Join(directory, "key.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(fixture.TLS.Certificates[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan Addresses, 1)
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, Config{Mode: "server", HTTPAddr: "127.0.0.1:0", StateDir: directory, TLSCert: cert, TLSKey: key}, func(a Addresses) { ready <- a })
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	select {
	case address := <-ready:
		client := fixture.Client()
		client.Timeout = time.Second
		response, err := client.Get("https://" + address.HTTP + "/owner")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != 200 {
			t.Fatal(response.StatusCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("TLS start timeout")
	}
}

func TestServerRejectsUnavailableOwnerStateAndTLSFiles(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "v1"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "v1", "owner.db"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := Serve(context.Background(), Config{Mode: "server", HTTPAddr: "127.0.0.1:0", StateDir: directory}, func(Addresses) {}); err == nil {
		t.Fatal("invalid owner state starts")
	}
	if err := Serve(context.Background(), Config{Mode: "server", HTTPAddr: "127.0.0.1:0", StateDir: t.TempDir(), TLSCert: "missing", TLSKey: "missing"}, func(Addresses) {}); err == nil {
		t.Fatal("missing TLS files start")
	}
}
