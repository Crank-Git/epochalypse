package command

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunShowsHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"-h"}, {"--help"}} {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut); code != 0 || !strings.Contains(out.String(), "Usage:") || errOut.Len() != 0 {
			t.Fatalf("Run(%q): code=%d stdout=%q stderr=%q", args, code, &out, &errOut)
		}
	}
}

func TestServiceFlagsRejectInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"server", "--bad"}, {"server", "extra"}} {
		if code := Run(args, io.Discard, io.Discard); code != 2 {
			t.Fatalf("%v: %d", args, code)
		}
	}
}

func TestServiceRejectsMissingAgentIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"agent", "--config", path, "--agent-id", ""}, io.Discard, io.Discard); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
}

func TestServiceStopsOnCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := runService(ctx, []string{"server", "--config", path, "--http-addr", "127.0.0.1:0", "--state-dir", t.TempDir()}, io.Discard); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
}

func TestEnvironmentOverridesDefaults(t *testing.T) {
	t.Setenv("EPOCHALYPSE_HTTP_ADDR", "127.0.0.1:9000")
	if got := envDefault("EPOCHALYPSE_HTTP_ADDR", "fallback"); got != "127.0.0.1:9000" {
		t.Fatal(got)
	}
}

func TestRunRejectsUnknownArguments(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"help", "extra"}, {"--bad"}} {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut); code != 2 || out.Len() != 0 || !strings.Contains(errOut.String(), "invalid command") {
			t.Fatalf("Run(%q): code=%d stdout=%q stderr=%q", args, code, &out, &errOut)
		}
	}
}

func TestRunRejectsMissingServiceConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"server", "--config", "missing.json"}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "open local configuration") {
		t.Fatalf("code=%d stderr=%q", code, &errOut)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRunRejectsOutputFailure(t *testing.T) {
	if code := Run(nil, failedWriter{}, &bytes.Buffer{}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if code := Run([]string{"bad"}, &bytes.Buffer{}, failedWriter{}); code != 1 {
		t.Fatalf("error exit code = %d, want 1", code)
	}
}

func FuzzRun(f *testing.F) {
	f.Add("help")
	f.Add("unknown")
	f.Fuzz(func(t *testing.T, arg string) {
		var out, errOut bytes.Buffer
		// Service modes must reject this flag before they can start a listener.
		code := Run([]string{arg, "--fuzz-invalid-flag"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d", code)
		}
	})
}
