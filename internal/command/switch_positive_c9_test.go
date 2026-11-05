package command

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func verifySwitchArmRetries(t *testing.T, binary string, paths switchFixturePaths, fixture *switchEndpointFixture) {
	t.Helper()
	intentBefore := readSwitchEvidence(t, paths.bundle+".intent")
	requestBefore := readSwitchEvidence(t, paths.bundle+".arm")
	bundleBefore := readSwitchEvidence(t, paths.bundle)
	args := switchArmArguments(paths)
	trust := map[string]string{"SSL_CERT_FILE": paths.certs}
	output := runSwitchExecutable(t, binary, args, trust)
	var retry struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(output, &retry); err != nil || retry.State != "armed" {
		t.Fatalf("exact arm retry output %q: %v", output, err)
	}
	assertSwitchEvidence(t, paths.bundle+".intent", intentBefore)
	assertSwitchEvidence(t, paths.bundle+".arm", requestBefore)
	assertSwitchEvidence(t, paths.bundle, bundleBefore)

	stopSwitchAgentProcess(fixture.processes[0])
	stdout, stderr, err := runSwitchExecutableResult(binary, args, trust)
	if err == nil {
		t.Fatal("arm retry succeeds without a response from every agent")
	}
	var retained struct {
		State string `json:"state"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &retained); decodeErr != nil || retained.State != "armed" {
		t.Fatalf("uncertain retry does not report retained armed evidence: stdout=%q error=%v", stdout.String(), decodeErr)
	}
	if !strings.Contains(stderr.String(), "arm remains uncertain") {
		t.Fatalf("uncertain retry diagnostic = %q", stderr.String())
	}
	assertSwitchEvidence(t, paths.bundle+".intent", intentBefore)
	assertSwitchEvidence(t, paths.bundle+".arm", requestBefore)
	assertSwitchEvidence(t, paths.bundle, bundleBefore)
}

func switchArmArguments(paths switchFixturePaths) []string {
	return []string{"switch", "arm", "--state-dir", paths.state, "--plan", paths.plan, "--owner-key", paths.ownerKey, "--bundle", paths.bundle, "--pins", paths.pins}
}

func readSwitchEvidence(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- The path belongs to the synthetic switch fixture.
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertSwitchEvidence(t *testing.T, path string, expected []byte) {
	t.Helper()
	if actual := readSwitchEvidence(t, path); !bytes.Equal(actual, expected) {
		t.Fatalf("retry changed durable evidence %q", path)
	}
}

func runSwitchExecutableResult(binary string, args []string, extra map[string]string) (*bytes.Buffer, *bytes.Buffer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...) // #nosec G204 G702 -- The test runs its built executable with fixed fixture arguments.
	environment := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, found := strings.Cut(entry, "=")
		if found {
			environment[key] = value
		}
	}
	for key, value := range extra {
		environment[key] = value
	}
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	return stdout, stderr, err
}
