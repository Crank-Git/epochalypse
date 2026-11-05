package command

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func TestReadPrivatePlanRejectsUnprotectedAndMalformedFiles(t *testing.T) {
	directory := t.TempDir()
	for name, content := range map[string]struct {
		mode os.FileMode
		body string
	}{
		"unprotected": {mode: 0o644, body: `{}`},
		"malformed":   {mode: 0o600, body: `{"private":"fixture-secret"}`},
		"trailing":    {mode: 0o600, body: `{} {}`},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(directory, name+".json")
			if err := os.WriteFile(path, []byte(content.body), content.mode); err != nil {
				t.Fatal(err)
			}
			if _, err := readPrivatePlan(path); err == nil || strings.Contains(err.Error(), "fixture-secret") {
				t.Fatalf("plan error=%v", err)
			}
		})
	}
}

func TestReadEndpointPinsRejectsMalformedMaps(t *testing.T) {
	directory := t.TempDir()
	for name, body := range map[string]string{
		"empty":        `{}`,
		"trailing":     `{} {}`,
		"short pin":    `{"https://agent.invalid":"00"}`,
		"bad hex":      `{"https://agent.invalid":"not-hex"}`,
		"empty target": `{"":"` + strings.Repeat("00", 32) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(directory, "pins-"+strings.ReplaceAll(name, " ", "-")+".json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readEndpointPins(path); err == nil {
				t.Fatal("malformed pin map was accepted")
			}
		})
	}

	validPath := filepath.Join(directory, "valid-pins.json")
	want := strings.Repeat("ab", 32)
	data, err := json.Marshal(map[string]string{"https://agent.invalid": want})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(validPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	pins, err := readEndpointPins(validPath)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := hex.DecodeString(want)
	var expected [32]byte
	copy(expected[:], decoded)
	if pins["https://agent.invalid"] != expected {
		t.Fatalf("pin = %x", pins["https://agent.invalid"])
	}
}

func TestSwitchExportRejectsInvalidInputsWithoutExposingThem(t *testing.T) {
	directory := t.TempDir()
	keyPath := filepath.Join(directory, "owner.pub")
	secret := "synthetic-owner-public-key"
	if err := os.WriteFile(keyPath, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(directory, "bundle.json")
	if err := os.WriteFile(bundlePath, []byte("synthetic-bundle-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"switch", "export", "--bundle", bundlePath, "--owner-public-key", keyPath, "--output", filepath.Join(directory, "out.json")}, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || strings.Contains(stderr.String(), secret) || strings.Contains(stderr.String(), "synthetic-bundle-secret") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestSwitchPrepareRejectsIncompleteAgentEnrollmentWithoutSecretOutput(t *testing.T) {
	directory := t.TempDir()
	draft, err := executeSwitch(directory, "", switches.Input{}, window.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(directory, "owner.key")
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(privateKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	contentPath := filepath.Join(directory, "content")
	if err := os.WriteFile(contentPath, []byte("synthetic-plaintext"), 0o600); err != nil {
		t.Fatal(err)
	}
	agentsPath := filepath.Join(directory, "agents.json")
	agents := prepareCommandAgents(t)
	data, err := json.Marshal(agents[:2])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentsPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"switch", "prepare", "--state-dir", directory, "--id", draft.ID, "--content", contentPath, "--agents", agentsPath, "--owner-key", keyPath, "--plan", filepath.Join(directory, "plan")}, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || strings.Contains(stdout.String()+stderr.String(), "synthetic") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
