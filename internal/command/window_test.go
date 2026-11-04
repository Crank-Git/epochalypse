package command

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func windowKey(t *testing.T) string {
	t.Helper()
	key := dns.NewDNSKEY("windows.example.", dns.ECDSAP256SHA256)
	key.Flags = 257
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic.private")
	for name, text := range map[string]string{path: key.PrivateKeyString(private), strings.TrimSuffix(path, ".private") + ".key": key.String()} {
		if err := os.WriteFile(name, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestWindowCommandCreatesAndInspectsSignedRecords(t *testing.T) {
	state, key := t.TempDir(), windowKey(t)
	inception := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	args := []string{"--state-dir", state, "--zone", "windows.example.", "--key-file", key}
	create := append([]string{"window", "create", "--content", "public CLI experiment", "--inception", inception.Format(time.RFC3339), "--expiration", inception.Add(time.Hour).Format(time.RFC3339)}, args...)
	var out, errOut bytes.Buffer
	if code := Run(create, &out, &errOut); code != 0 {
		t.Fatalf("window create: exit=%d error=%s", code, &errOut)
	}
	var created window.Window
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Content != "public CLI experiment" || !created.Inception.Equal(inception) {
		t.Fatal("creation changes explicit input")
	}
	if _, _, _, err := created.Records(); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	inspect := append([]string{"window", "inspect", "--id", created.ID}, args...)
	if code := Run(inspect, &out, &errOut); code != 0 {
		t.Fatalf("window inspect: exit=%d error=%s", code, &errOut)
	}
	var inspected window.Window
	if err := json.Unmarshal(out.Bytes(), &inspected); err != nil {
		t.Fatal(err)
	}
	if inspected != created {
		t.Fatal("inspection changes the signed record")
	}
}

func TestWindowCommandRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"window"}, {"window", "unknown"}, {"window", "create", "--bad"}, {"window", "create"}, {"window", "inspect", "--zone", "windows.example.", "--key-file", "missing"}, {"window", "create", "extra"}} {
		if code := Run(args, io.Discard, io.Discard); code != 2 {
			t.Fatalf("arguments %q return %d", args, code)
		}
	}
}

func TestWindowCommandReportsDependencyAndOutputFailures(t *testing.T) {
	key := windowKey(t)
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"window", "create", "--zone", "windows.example.", "--key-file", "missing.private"},
		{"window", "create", "--zone", "windows.example.", "--key-file", key, "--state-dir", filepath.Join(blocked, "state")},
		{"window", "create", "--zone", "windows.example.", "--key-file", key, "--state-dir", t.TempDir()},
		{"window", "inspect", "--zone", "windows.example.", "--key-file", key, "--state-dir", t.TempDir(), "--id", strings.Repeat("a", 32)},
	} {
		if code := Run(args, io.Discard, io.Discard); code != 1 {
			t.Fatalf("failure returns %d", code)
		}
	}
	inception := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	args := []string{"window", "create", "--zone", "windows.example.", "--key-file", key, "--state-dir", t.TempDir(), "--inception", inception.Format(time.RFC3339), "--expiration", inception.Add(time.Hour).Format(time.RFC3339)}
	if code := Run(args, failedWriter{}, io.Discard); code != 1 {
		t.Fatal("failed output returns success")
	}
}

func TestWindowCommandRejectsInvalidRetentionLimits(t *testing.T) {
	for _, values := range [][2]string{{"0", "1"}, {"1", "0"}, {"bad", "1"}, {"1", "bad"}, {"-1", "1"}} {
		for _, command := range [][]string{{"window", "create", "--zone", "windows.example.", "--key-file", "missing.private"}, {"server"}} {
			args := append(command, "--max-objects", values[0], "--quota-bytes", values[1])
			if code := Run(args, io.Discard, io.Discard); code != 2 {
				t.Fatalf("limits return %d", code)
			}
		}
	}
}

func TestServiceReadsTheConfiguredWindowZone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"zone":"windows.example."}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EPOCHALYPSE_KEY_FILE", windowKey(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	args := []string{"server", "--config", path, "--state-dir", t.TempDir(), "--http-addr", "127.0.0.1:0", "--dns-addr", "127.0.0.1:0", "--local-lab"}
	if code := runService(ctx, args, io.Discard); code != 0 {
		t.Fatalf("configured server returns %d", code)
	}
}
