package command

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/agent"
	"github.com/Crank-Git/epochalypse/internal/switchclient"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func verifyStandaloneSwitchRecovery(t *testing.T, binary string, paths switchFixturePaths, plan switchclient.Plan, fixture *switchEndpointFixture, owner ed25519.PublicKey) {
	t.Helper()
	// Controlled evidence tests the executable recovery path without a production clock override.
	for _, i := range []int{1, 2} {
		publishSwitchFixture(t, paths, plan, fixture, owner, i)
		restartReleasedSwitchAgent(t, binary, paths, fixture, i)
	}
	if err := os.Rename(paths.state, paths.state+".offline"); err != nil {
		t.Fatal(err)
	}
	args := []string{"switch", "recover", "--bundle", paths.bundle, "--owner-public-key", paths.ownerPublic, "--pins", paths.pins}
	output := runSwitchExecutable(t, binary, args, map[string]string{"SSL_CERT_FILE": paths.certs})
	if string(output) != "positive executable switch fixture" {
		t.Fatalf("standalone recovery content = %q", output)
	}
	stopSwitchAgentProcess(fixture.processes[1])
	stdout, _, err := runSwitchExecutableResult(binary, args, map[string]string{"SSL_CERT_FILE": paths.certs})
	if err == nil || stdout.Len() != 0 {
		t.Fatalf("one agent exposes plaintext: %q %v", stdout.String(), err)
	}
}

func publishSwitchFixture(t *testing.T, paths switchFixturePaths, plan switchclient.Plan, fixture *switchEndpointFixture, owner ed25519.PublicKey, i int) {
	t.Helper()
	a := fixture.agents[i]
	root := filepath.Join(paths.root, a.ID)
	journal, err := agent.Open(agent.Config{Directory: filepath.Join(root, "authority"), CheckpointDirectory: filepath.Join(root, "checkpoint"), ExpectedVolumeID: "test-" + a.ID})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()
	state, err := journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := switchwire.ReadEnvelope(bytes.NewReader(state.AcceptedRequest))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := switchwire.DecodeFrame(accepted.Body)
	if err != nil {
		t.Fatal(err)
	}
	transition, err := agent.NewTransitions(agent.TransitionConfig{Journal: journal, OwnerKey: owner, EnrollmentBody: plan.Certificate, EnrollmentSignature: plan.CertificateSignature, AgentID: a.ID, SigningKey: fixture.signingKeys[i], AgeIdentity: fixture.identities[i].String(), ReadClock: func() (agent.Reading, error) {
		return agent.AssessClock(agent.Assessment{Epoch: "recovery", Implementation: "synthetic", Provenance: "test", SourceIdentities: []string{"a", "b"}, ReferenceEvidence: "test", IndependenceEvidence: "test", LeapPolicy: "UTC"}, agent.Capture{Epoch: "recovery", UTCSeconds: frame.State.Deadline + int64(frame.State.Grace) + 2}) // #nosec G115 -- DecodeFrame bounds deadline plus grace below MaxInt64.
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := transition.Publish(); err != nil {
		t.Fatal(err)
	}
}

func restartReleasedSwitchAgent(t *testing.T, binary string, paths switchFixturePaths, fixture *switchEndpointFixture, i int) {
	t.Helper()
	root := filepath.Join(paths.root, fixture.agents[i].ID)
	endpoint := strings.TrimPrefix(fixture.agents[i].Endpoint, "https://")
	args := []string{"agent", "serve", "--config", filepath.Join(root, "runtime.json"), "--http-addr", endpoint, "--tls-cert", filepath.Join(root, "tls.crt"), "--tls-key", filepath.Join(root, "tls.key")}
	if i != 2 {
		args = append(args, "--lab-only-synthetic-clock")
	}
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, binary, args...)       // #nosec G204 G702 -- The test runs its built executable with fixed fixture arguments.
	log, err := os.Create(filepath.Join(root, "recovery.log")) // #nosec G304 -- The test owns this temporary log.
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	command.Stdout, command.Stderr = log, log
	if err := command.Start(); err != nil {
		cancel()
		_ = log.Close()
		t.Fatal(err)
	}
	process := &switchAgentProcess{command: command, cancel: cancel, done: make(chan error, 1)}
	fixture.processes[i] = process
	t.Cleanup(func() { stopSwitchAgentProcess(process) })
	go func() { process.done <- command.Wait() }()
	waitForAgentListener(t, endpoint, filepath.Join(root, "recovery.log"))
	_ = log.Close()
}
