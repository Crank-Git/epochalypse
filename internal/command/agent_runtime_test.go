package command

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/agent"
)

func TestAgentRuntimeRequiresProtectedConfiguration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"agent", "serve", "--lab-only-synthetic-clock"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "agent runtime configuration is required") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestAgentRuntimeRejectsPublicConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	if err := os.WriteFile(path, []byte(`{"age_identity":"private-fixture"}`), 0o644); err != nil { // #nosec G306 -- The test must reject this public synthetic configuration.
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"agent", "initialize", "--config", path}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "owner-only regular file") || strings.Contains(stderr.String(), "private-fixture") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestAgentRuntimeRejectsUnknownAndTrailingConfiguration(t *testing.T) {
	for _, input := range []string{`{"unknown":"private-fixture"}`, `{} {}`} {
		t.Run(input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent.json")
			if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readAgentRuntimeSettings(path); err == nil || strings.Contains(err.Error(), "private-fixture") {
				t.Fatalf("configuration error=%v", err)
			}
		})
	}
}

func TestAgentInitializePreservesKnownAuthority(t *testing.T) {
	root := t.TempDir()
	authority := filepath.Join(root, "authority")
	if err := os.Mkdir(authority, 0o700); err != nil {
		t.Fatal(err)
	}
	settings := agentRuntimeSettings{Authority: agent.Config{Directory: authority, CheckpointDirectory: filepath.Join(root, "checkpoint"), ExpectedVolumeID: "explicit-test-volume", LabOnlySyntheticStorage: true}}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "agent.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"agent", "initialize", "--config", path}
	if code := Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("initialize code=%d stderr=%q", code, stderr.String())
	}
	if code := Run(args, &stdout, &stderr); code != 1 {
		t.Fatalf("reinitialize code=%d stderr=%q", code, stderr.String())
	}
	journal, err := agent.Open(settings.Authority)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentServeRejectsMissingTLSIdentity(t *testing.T) {
	err := serveAgentRuntime(context.Background(), agentRuntimeSettings{}, "127.0.0.1:0", "", "")
	if err == nil || !strings.Contains(err.Error(), "load agent HTTPS identity") {
		t.Fatalf("serve error=%v", err)
	}
}

func TestAgentRuntimeRejectsProductionClockClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"agent", "serve", "--config", path}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "explicit lab clock mode is required") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestAgentRuntimeRejectsConflictingClockModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	if err := os.WriteFile(path, []byte(`{"clock_profile":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"agent", "serve", "--config", path, "--lab-only-synthetic-clock"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "configure exactly one clock mode") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
