package command

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/agent"
	"github.com/Crank-Git/epochalypse/internal/switchclient"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestCLIArmsSwitchWithThreeDurableTLSAgents(t *testing.T) {
	binary := os.Getenv("EPOCHALYPSE_SWITCH_ARM_BINARY")
	if binary == "" {
		return
	}
	root := t.TempDir()
	ownerPublic, ownerPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	paths := newSwitchFixturePaths(root)
	writeSwitchOwnerFiles(t, paths, ownerPublic, ownerPrivate)
	fixture := newSwitchTLSServers(t)
	describeSwitchAgents(t, fixture)
	writeSwitchEnrollmentFiles(t, paths, fixture)
	createAndPrepareSwitch(t, binary, paths)
	plan := readSwitchPlan(t, paths.plan)
	startSwitchAgentProcesses(t, binary, paths, plan, fixture, ownerPublic)
	armAndExportSwitch(t, binary, paths)
	verifySwitchArmRetries(t, binary, paths, fixture)
	stopSwitchAgentProcesses(fixture)
	assertAgentsArmed(t, paths, fixture)
}

type switchFixturePaths struct{ root, state, ownerKey, ownerPublic, plan, bundle, content, agents, pins, certs string }

func newSwitchFixturePaths(root string) switchFixturePaths {
	return switchFixturePaths{root: root, state: filepath.Join(root, "state"), ownerKey: filepath.Join(root, "owner.key"), ownerPublic: filepath.Join(root, "owner.pub"), plan: filepath.Join(root, "plan.json"), bundle: filepath.Join(root, "recovery.json"), content: filepath.Join(root, "content.txt"), agents: filepath.Join(root, "agents.json"), pins: filepath.Join(root, "pins.json"), certs: filepath.Join(root, "roots.pem")}
}

type switchEndpointFixture struct {
	servers     []*httptest.Server
	agents      [3]switchwire.Agent
	identities  [3]*age.X25519Identity
	signingKeys [3]ed25519.PrivateKey
	processes   [3]*switchAgentProcess
}

type switchAgentProcess struct {
	command *exec.Cmd
	cancel  context.CancelFunc
	done    chan error
	stopped bool
}

func writeSwitchOwnerFiles(t *testing.T, paths switchFixturePaths, public ed25519.PublicKey, private ed25519.PrivateKey) {
	t.Helper()
	writeFixtureFile(t, paths.ownerKey, []byte(base64.StdEncoding.EncodeToString(private)), 0o600)
	writeFixtureFile(t, paths.ownerPublic, []byte(base64.StdEncoding.EncodeToString(public)), 0o600)
	writeFixtureFile(t, paths.content, []byte("positive executable switch fixture"), 0o600)
}

func newSwitchTLSServers(t *testing.T) *switchEndpointFixture {
	t.Helper()
	fixture := &switchEndpointFixture{servers: make([]*httptest.Server, 3)}
	for i := range fixture.servers {
		fixture.servers[i] = httptest.NewTLSServer(http.NotFoundHandler())
		t.Cleanup(fixture.servers[i].Close)
	}
	return fixture
}

func describeSwitchAgents(t *testing.T, fixture *switchEndpointFixture) {
	t.Helper()
	for i, id := range []string{"agent-a", "agent-b", "agent-c"} {
		public, private, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		fixture.signingKeys[i] = private
		fixture.identities[i], err = age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		fixture.agents[i] = switchwire.Agent{ID: id, Endpoint: fixture.servers[i].URL, Recipient: fixture.identities[i].Recipient().String()}
		copy(fixture.agents[i].SigningKey[:], public)
	}
}

func writeSwitchEnrollmentFiles(t *testing.T, paths switchFixturePaths, fixture *switchEndpointFixture) {
	t.Helper()
	var certificates strings.Builder
	pins := make(map[string]string, len(fixture.servers))
	for _, server := range fixture.servers {
		certificates.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
		pin := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
		pins[server.URL] = hex.EncodeToString(pin[:])
	}
	writeFixtureFile(t, paths.certs, []byte(certificates.String()), 0o600)
	writeJSONFixture(t, paths.agents, fixture.agents)
	writeJSONFixture(t, paths.pins, pins)
}

func writeJSONFixture(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, path, data, 0o600)
}

func createAndPrepareSwitch(t *testing.T, binary string, paths switchFixturePaths) {
	t.Helper()
	trust := map[string]string{"SSL_CERT_FILE": paths.certs}
	created := runSwitchExecutable(t, binary, []string{"switch", "create", "--state-dir", paths.state}, trust)
	var draft struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created, &draft); err != nil || draft.ID == "" {
		t.Fatalf("create draft output %q: %v", created, err)
	}
	args := []string{"switch", "prepare", "--state-dir", paths.state, "--id", draft.ID, "--content", paths.content, "--agents", paths.agents, "--owner-key", paths.ownerKey, "--plan", paths.plan}
	runSwitchExecutable(t, binary, args, trust)
}

func readSwitchPlan(t *testing.T, path string) switchclient.Plan {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- The test reads its plan from its temporary fixture directory.
	if err != nil {
		t.Fatal(err)
	}
	var plan switchclient.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func startSwitchAgentProcesses(t *testing.T, binary string, paths switchFixturePaths, plan switchclient.Plan, fixture *switchEndpointFixture, owner ed25519.PublicKey) {
	t.Helper()
	for i, enrolled := range fixture.agents {
		root := filepath.Join(paths.root, enrolled.ID)
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		authority := filepath.Join(root, "authority")
		if err := os.Mkdir(authority, 0o700); err != nil {
			t.Fatal(err)
		}
		server := fixture.servers[i]
		server.Close()
		certificate := server.TLS.Certificates[0]
		certificatePath := filepath.Join(root, "tls.crt")
		privateKeyPath := filepath.Join(root, "tls.key")
		privateKey, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0o600)
		writeFixtureFile(t, privateKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey}), 0o600)
		endpoint := strings.TrimPrefix(enrolled.Endpoint, "https://")
		settingsPath := filepath.Join(root, "runtime.json")
		settings := map[string]any{"authority": agent.Config{Directory: authority, CheckpointDirectory: filepath.Join(root, "checkpoint"), ExpectedVolumeID: "test-" + enrolled.ID}, "agent_id": enrolled.ID, "owner_key": owner, "certificate": plan.Certificate, "certificate_signature": plan.CertificateSignature, "signing_key": fixture.signingKeys[i], "age_identity": fixture.identities[i].String()}
		clockArgs := []string{"--lab-only-synthetic-clock"}
		if i == 2 {
			settings["clock_profile"] = writeSwitchClockProfile(t, root)
			clockArgs = nil
		}
		writeJSONFixture(t, settingsPath, settings)
		runSwitchExecutable(t, binary, []string{"agent", "initialize", "--config", settingsPath}, nil)
		ctx, cancel := context.WithCancel(context.Background())
		commandArgs := append([]string{"agent", "serve", "--config", settingsPath, "--http-addr", endpoint, "--tls-cert", certificatePath, "--tls-key", privateKeyPath}, clockArgs...)
		command := exec.CommandContext(ctx, binary, commandArgs...) // #nosec G204 G702 -- The test runs its built executable with fixed fixture arguments.
		log, err := os.Create(filepath.Join(root, "agent.log"))     // #nosec G304 -- The log path is inside the test's temporary fixture directory.
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		command.Stdout, command.Stderr = log, log
		if err := command.Start(); err != nil {
			_ = log.Close()
			cancel()
			t.Fatal(err)
		}
		process := &switchAgentProcess{command: command, cancel: cancel}
		process.done = make(chan error, 1)
		fixture.processes[i] = process
		t.Cleanup(func() { stopSwitchAgentProcess(process) })
		go func() { process.done <- command.Wait() }()
		waitForAgentListener(t, endpoint, filepath.Join(root, "agent.log"))
		_ = log.Close()
	}
}

func waitForAgentListener(t *testing.T, endpoint, logPath string) {
	t.Helper()
	address, err := net.ResolveTCPAddr("tcp", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		connection, dialErr := net.DialTimeout("tcp", address.String(), 100*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, err := os.ReadFile(logPath) // #nosec G304 -- The log path is inside the test's temporary fixture directory.
	if err != nil {
		t.Fatalf("agent listener %s did not start; read log: %v", endpoint, err)
	}
	t.Fatalf("agent listener %s did not start: %s", endpoint, data)
}

func stopSwitchAgentProcesses(fixture *switchEndpointFixture) {
	for _, process := range fixture.processes {
		stopSwitchAgentProcess(process)
	}
}

func stopSwitchAgentProcess(process *switchAgentProcess) {
	if process == nil || process.command == nil || process.stopped {
		return
	}
	process.stopped = true
	_ = process.command.Process.Signal(os.Interrupt)
	select {
	case err := <-process.done:
		_ = err
	case <-time.After(3 * time.Second):
		process.cancel()
		err := <-process.done
		_ = err
	}
	process.cancel()
}

func armAndExportSwitch(t *testing.T, binary string, paths switchFixturePaths) {
	t.Helper()
	trust := map[string]string{"SSL_CERT_FILE": paths.certs}
	args := []string{"switch", "arm", "--state-dir", paths.state, "--plan", paths.plan, "--owner-key", paths.ownerKey, "--bundle", paths.bundle, "--pins", paths.pins}
	armed := runSwitchExecutable(t, binary, args, trust)
	var record struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(armed, &record); err != nil || record.State != "armed" {
		t.Fatalf("arm output %q: %v", armed, err)
	}
	if _, err := os.Stat(paths.bundle); err != nil {
		t.Fatalf("arm did not export the recovery bundle: %v", err)
	}
	publicPath := filepath.Join(paths.root, "owner.pub")
	outputPath := filepath.Join(paths.root, "validated-recovery.json")
	exportArgs := []string{"switch", "export", "--bundle", paths.bundle, "--owner-public-key", publicPath, "--output", outputPath}
	runSwitchExecutable(t, binary, exportArgs, nil)
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("export did not write the validated bundle: %v", err)
	}
}

func assertAgentsArmed(t *testing.T, paths switchFixturePaths, fixture *switchEndpointFixture) {
	t.Helper()
	for i, enrolled := range fixture.agents {
		config := agent.Config{Directory: filepath.Join(paths.root, enrolled.ID, "authority"), CheckpointDirectory: filepath.Join(paths.root, enrolled.ID, "checkpoint"), ExpectedVolumeID: "test-" + enrolled.ID}
		journal, err := agent.Open(config)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := journal.Snapshot()
		_ = journal.Close()
		if err != nil || snapshot.Phase != switchwire.PhaseArmed || len(snapshot.Receipt) == 0 {
			t.Fatalf("agent %d did not retain an armed receipt: phase=%d err=%v", i, snapshot.Phase, err)
		}
	}
}

func runSwitchExecutable(t *testing.T, binary string, args []string, extra map[string]string) []byte {
	t.Helper()
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
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI %v failed: %v\n%s", args, err, output)
	}
	return output
}

func writeFixtureFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
