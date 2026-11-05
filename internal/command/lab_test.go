package command

import (
	"bytes"
	"context"
	"github.com/Crank-Git/epochalypse/internal/service"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLabVerifyReportsUnsupportedPublicChain(t *testing.T) {
	var out, stderr bytes.Buffer
	code := Run([]string{"lab", "verify", "--zone", "windows.example.", "--address", "127.0.0.1:1053"}, &out, &stderr)
	if code != 1 || !strings.Contains(out.String(), `"status":"indeterminate"`) || !strings.Contains(out.String(), `"trust_source":"none"`) {
		t.Fatalf("code=%d out=%s error=%s", code, &out, &stderr)
	}
}

func TestLabVerifyUsesTheLiveAuthority(t *testing.T) {
	key := windowKey(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	addresses := make(chan service.Addresses, 1)
	go func() {
		done <- service.Serve(ctx, service.Config{Mode: "server", Environment: "lab", HTTPAddr: "127.0.0.1:0", DNSAddr: "127.0.0.1:0", StateDir: t.TempDir(), LocalLab: true, Zone: "windows.example.", KeyFile: key, NameServer: "ns.lab.invalid.", PublicURL: "http://localhost:8080"}, func(a service.Addresses) { addresses <- a })
	}()
	var a service.Addresses
	select {
	case a = <-addresses:
	case err := <-done:
		cancel()
		t.Fatal(err)
	}
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	args := []string{"lab", "verify", "--zone", "windows.example.", "--address", a.DNS, "--anchor-file", strings.TrimSuffix(key, ".private") + ".key"}
	var out, stderr bytes.Buffer
	if code := Run(args, &out, &stderr); code != 0 || !strings.Contains(out.String(), `"status":"verified"`) {
		t.Fatalf("%d %s %s", code, &out, &stderr)
	}
	if code := Run(args, failedWriter{}, io.Discard); code != 1 {
		t.Fatalf("output failure: %d", code)
	}
}

func TestLabVerifyRejectsInvalidInput(t *testing.T) {
	for _, args := range [][]string{{"lab"}, {"lab", "bad"}, {"lab", "verify", "--bad"}, {"lab", "verify"}, {"lab", "verify", "--zone", "windows.example.", "extra"}, {"lab", "verify", "--zone", "windows.example.", "--anchor-file", "missing"}} {
		if code := Run(args, io.Discard, io.Discard); code != 2 {
			t.Fatalf("%q: %d", args, code)
		}
	}
	for _, material := range []string{"SECRET PRIVATE CONTENT", strings.Repeat("x", 4097)} {
		path := filepath.Join(t.TempDir(), "anchor")
		if err := os.WriteFile(path, []byte(material), 0600); err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		code := Run([]string{"lab", "verify", "--zone", "windows.example.", "--anchor-file", path}, io.Discard, &stderr)
		if code != 2 || strings.Contains(stderr.String(), "SECRET") {
			t.Fatalf("%d %s", code, &stderr)
		}
	}
	if _, err := readAnchor(t.TempDir(), "windows.example."); err == nil {
		t.Fatal("accepts a directory")
	}
	if code := Run([]string{"lab", "verify", "--zone", "windows.example."}, failedWriter{}, io.Discard); code != 1 {
		t.Fatalf("output failure: %d", code)
	}
}
