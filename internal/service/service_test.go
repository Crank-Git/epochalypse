package service

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHealthReturns200(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan Addresses, 1)
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, Config{Mode: "server", HTTPAddr: "127.0.0.1:0", StateDir: t.TempDir()}, func(a Addresses) { ready <- a })
	}()
	var addr Addresses
	select {
	case addr = <-ready:
	case err := <-done:
		t.Fatalf("start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("start timeout")
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + addr.HTTP + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || string(body) != "ok\n" {
		t.Fatalf("health: %d %q", response.StatusCode, body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}

func TestConfigRejectsIncompleteSettings(t *testing.T) {
	for _, config := range []Config{
		{Mode: "other"},
		{Mode: "server"},
		{Mode: "server", HTTPAddr: "127.0.0.1:0"},
		{Mode: "dns", HTTPAddr: "127.0.0.1:0", StateDir: t.TempDir()},
	} {
		if err := config.Validate(); err == nil {
			t.Fatalf("accepted %+v", config)
		}
		if err := Serve(context.Background(), config, func(Addresses) {}); err == nil {
			t.Fatal("invalid service started")
		}
	}
}

func TestReadConfigRejectsInvalidDocuments(t *testing.T) {
	if err := ReadConfig(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file accepted")
	}
	for _, document := range []string{`{"version":1}`, `bad`, `{"version":2}`, `{"unknown":1}`, `{"version":1} {}`, `{"version":1} bad`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(document), 0600); err != nil {
			t.Fatal(err)
		}
		err := ReadConfig(path)
		if (err == nil) != (document == `{"version":1}`) {
			t.Fatalf("document %q: %v", document, err)
		}
	}
}

func TestServiceRejectsInvalidStateDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Serve(context.Background(), Config{Mode: "server", HTTPAddr: "127.0.0.1:0", StateDir: path}, func(Addresses) {}); err == nil {
		t.Fatal("invalid state directory accepted")
	}
}

func TestServiceRejectsOccupiedDNSPorts(t *testing.T) {
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tcp.Close() }()
	if err := Serve(context.Background(), Config{Mode: "dns", HTTPAddr: "127.0.0.1:0", DNSAddr: tcp.Addr().String(), StateDir: t.TempDir()}, func(Addresses) {}); err == nil {
		t.Fatal("occupied TCP DNS port accepted")
	}
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = udp.Close() }()
	if _, err := openDNS(context.Background(), udp.LocalAddr().String()); err == nil {
		t.Fatal("occupied UDP DNS port accepted")
	}
}

func TestConfigRejectsMissingAgentIdentity(t *testing.T) {
	if err := (Config{Mode: "agent", HTTPAddr: "127.0.0.1:0", StateDir: filepath.Join(t.TempDir(), "v1")}).Validate(); err == nil {
		t.Fatal("missing agent identity accepted")
	}
}

func TestServiceRejectsOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := Serve(context.Background(), Config{Mode: "server", HTTPAddr: listener.Addr().String(), StateDir: t.TempDir()}, func(Addresses) {}); err == nil {
		t.Fatal("occupied port accepted")
	}
}
