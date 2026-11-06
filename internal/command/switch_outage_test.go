package command

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchclient"
)

func TestSwitchOutageRecoversAfterMainAndEachAgentStop(t *testing.T) {
	binary := os.Getenv("EPOCHALYPSE_SWITCH_ARM_BINARY")
	if binary == "" {
		return
	}
	for unavailable := range 3 {
		t.Run(fmt.Sprintf("agent-%d", unavailable), func(t *testing.T) {
			runSwitchOutage(t, binary, unavailable)
		})
	}
}

func runSwitchOutage(t *testing.T, binary string, unavailable int) {
	t.Helper()
	paths := newSwitchFixturePaths(t.TempDir())
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	writeSwitchOwnerFiles(t, paths, public, private)
	fixture := newSwitchTLSServers(t)
	describeSwitchAgents(t, fixture)
	writeSwitchEnrollmentFiles(t, paths, fixture)
	createAndPrepareSwitch(t, binary, paths)
	plan := readSwitchPlan(t, paths.plan)
	startSwitchAgentProcesses(t, binary, paths, plan, fixture, public)
	armAndExportSwitch(t, binary, paths)
	stopSwitchAgentProcesses(fixture)
	for i := range 3 {
		writeOutageClock(t, paths, fixture, i, time.Now().Unix())
		startOutageAgent(t, paths, fixture, i)
	}
	main := startOutageMain(t, binary, paths)
	trust := map[string]string{"SSL_CERT_FILE": paths.certs}
	renewal := loseSwitchRenewalReceipts(t, binary, paths, fixture, trust)
	stopSwitchAgentProcess(main)
	stopSwitchAgentProcess(fixture.processes[unavailable])
	assertOutageListenerStopped(t, main.command.Args[mainAddressIndex])
	assertOutageListenerStopped(t, strings.TrimPrefix(fixture.agents[unavailable].Endpoint, "https://"))
	assertOutageRenewed(t, paths, fixture, unavailable, renewal.Bmax)
	assertOutageClockQuarantine(t, paths, fixture, unavailable)
	for _, path := range []string{paths.state, paths.ownerKey, paths.plan, paths.content, paths.bundle + ".checkins"} {
		if err := os.Rename(path, path+".lost"); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		if i != unavailable {
			stopSwitchAgentProcess(fixture.processes[i])
			assertOutageRenewed(t, paths, fixture, i, renewal.Bmax)
			startOutageAgent(t, paths, fixture, i)
		}
	}
	verifyOutageRelease(t, binary, paths, fixture, unavailable, renewal, trust)
}

func loseSwitchRenewalReceipts(t *testing.T, binary string, paths switchFixturePaths, fixture *switchEndpointFixture, trust map[string]string) switchclient.Renewal {
	t.Helper()
	verifySwitchCheckinConfirmed(t, binary, paths, trust)
	time.Sleep(time.Second)
	for _, enrolled := range fixture.agents {
		writeFixtureFile(t, filepath.Join(paths.root, enrolled.ID, "lose-receipts"), nil, 0o600)
	}
	stdout, _, requestErr := runSwitchExecutableResult(binary, switchCheckinArguments(paths), trust)
	var renewal switchclient.Renewal
	if err := json.Unmarshal(stdout.Bytes(), &renewal); err != nil || requestErr == nil || renewal.Confirmed || len(renewal.Receipts) != 0 || renewal.Bmax <= renewal.B0 {
		t.Fatalf("lost receipts hide uncertainty: %s %v %v", stdout.String(), requestErr, err)
	}
	intent := readSwitchEvidence(t, paths.bundle+".checkins/3.request")
	stdout, _, _ = runSwitchExecutableResult(binary, switchCheckinArguments(paths), trust)
	assertSwitchEvidence(t, paths.bundle+".checkins/3.request", intent)
	var retried switchclient.Renewal
	if err := json.Unmarshal(stdout.Bytes(), &retried); err != nil || retried.Bmax != renewal.Bmax || retried.B0 != renewal.B0 {
		t.Fatalf("client restart changes issued bounds: %s %v", stdout.String(), err)
	}
	return renewal
}

func verifyOutageRelease(t *testing.T, binary string, paths switchFixturePaths, fixture *switchEndpointFixture, unavailable int, renewal switchclient.Renewal, trust map[string]string) {
	t.Helper()
	args := []string{"switch", "recover", "--bundle", paths.bundle, "--owner-public-key", paths.ownerPublic, "--pins", paths.pins}
	before, _, err := runSwitchExecutableResult(binary, args, trust)
	if err == nil || before.Len() != 0 {
		t.Fatalf("recovery exposes content before release: %q %v", before.String(), err)
	}
	started := time.Now()
	for i := range 3 {
		if i != unavailable {
			writeOutageClock(t, paths, fixture, i, renewal.Bmax+2)
		}
	}
	// The scheduler must publish before recovery makes a public request.
	time.Sleep(6 * time.Second)
	assertOutageScheduledRelease(t, paths, fixture, unavailable)
	output := runSwitchExecutable(t, binary, args, trust)
	elapsed := time.Since(started)
	if string(output) != "positive executable switch fixture" || elapsed > 32*time.Second {
		t.Fatalf("recovery exceeds Bmax+34: elapsed=%s content=%q", elapsed, output)
	}
	t.Logf("outage=%s B0=%d Bmax=%d controlled_UTC=%d E=34 elapsed=%s total_after_Bmax=%s receipts=lost owner=lost main=stopped", fixture.agents[unavailable].ID, renewal.B0, renewal.Bmax, renewal.Bmax+2, elapsed, elapsed+2*time.Second)
	assertOutageObserverCopies(t, paths, fixture, unavailable)
}

type outageClock struct {
	UTCSeconds   int64
	RecordedNano int64
}

func writeOutageClock(t *testing.T, paths switchFixturePaths, fixture *switchEndpointFixture, i int, seconds int64) {
	t.Helper()
	path := filepath.Join(paths.root, fixture.agents[i].ID, "clock")
	writeJSONFixture(t, path+".next", outageClock{UTCSeconds: seconds, RecordedNano: time.Now().UnixNano()})
	if err := os.Rename(path+".next", path); err != nil {
		t.Fatal(err)
	}
}

func startOutageAgent(t *testing.T, paths switchFixturePaths, fixture *switchEndpointFixture, i int) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(paths.root, fixture.agents[i].ID)
	address := strings.TrimPrefix(fixture.agents[i].Endpoint, "https://")
	fixture.processes[i] = startOutageProcess(t, binary, []string{"-test.run=^TestSwitchOutageAgentProcess$", "-test.timeout=90s"},
		[]string{"EPOCHALYPSE_OUTAGE_AGENT=" + root, "EPOCHALYPSE_OUTAGE_ADDRESS=" + address}, filepath.Join(root, "outage.log"), address)
}

const mainAddressIndex = 8

func startOutageMain(t *testing.T, binary string, paths switchFixturePaths) *switchAgentProcess {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	config := filepath.Join(paths.root, "main.json")
	writeFixtureFile(t, config, []byte(`{"version":1}`), 0o600)
	process := startOutageProcess(t, binary, []string{"server", "--config", config, "--state-dir", paths.state, "--local-lab", "--http-addr", address, "--dns-addr", "127.0.0.1:0"}, nil, filepath.Join(paths.root, "main.log"), address)
	commit := os.Getenv("EPOCHALYPSE_OUTAGE_COMMIT")
	if len(commit) != 40 {
		t.Fatal("outage drill requires the exact deployed commit")
	}
	output := runSwitchExecutable(t, binary, []string{"status", "--endpoints", "http://" + address, commit}, nil)
	if !strings.Contains(string(output), "success local "+commit) {
		t.Fatalf("deployed commit differs: %s", output)
	}
	return process
}

func startOutageProcess(t *testing.T, binary string, args, environment []string, logPath, address string) *switchAgentProcess {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, binary, args...) // #nosec G204 G702 -- The drill runs its own executable with fixture arguments.
	command.Env = append(os.Environ(), environment...)
	log, err := os.Create(logPath) // #nosec G304 -- The drill owns its temporary log.
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
	go func() { process.done <- command.Wait() }()
	t.Cleanup(func() { stopSwitchAgentProcess(process) })
	waitForAgentListener(t, address, logPath)
	_ = log.Close()
	return process
}

func assertOutageListenerStopped(t *testing.T, address string) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Fatalf("outage process still listens at %s", address)
	}
}
