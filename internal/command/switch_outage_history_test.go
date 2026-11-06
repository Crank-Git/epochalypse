package command

import (
	"bytes"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/agent"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func outageSnapshot(t *testing.T, paths switchFixturePaths, fixture *switchEndpointFixture, i int) agent.State {
	t.Helper()
	root := filepath.Join(paths.root, fixture.agents[i].ID)
	journal, err := agent.Open(agent.Config{Directory: filepath.Join(root, "authority"), CheckpointDirectory: filepath.Join(root, "checkpoint"), ExpectedVolumeID: "test-" + fixture.agents[i].ID})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()
	state, err := journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func assertOutageRenewed(t *testing.T, paths switchFixturePaths, fixture *switchEndpointFixture, i int, boundary int64) {
	t.Helper()
	state := outageSnapshot(t, paths, fixture, i)
	envelope, err := switchwire.ReadEnvelope(bytes.NewReader(state.AcceptedRequest))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := switchwire.DecodeFrame(envelope.Body)
	if err != nil || frame.State.Sequence != 3 || frame.State.Deadline+int64(frame.State.Grace) != boundary || state.PossibleDisclosure {
		t.Fatalf("lost receipt does not retain the issued renewal before release: %v", err)
	} // #nosec G115 -- DecodeFrame bounds the signed deadline plus grace.
}

func assertOutageScheduledRelease(t *testing.T, paths switchFixturePaths, fixture *switchEndpointFixture, unavailable int) {
	t.Helper()
	for i := range 3 {
		if i == unavailable {
			continue
		}
		stopSwitchAgentProcess(fixture.processes[i])
		state := outageSnapshot(t, paths, fixture, i)
		if !state.PossibleDisclosure || len(state.ReleaseEnvelope) == 0 {
			t.Fatal("scheduler does not persist release before public retrieval")
		}
		// A released agent needs no new clock authority after restart.
		writeFixtureFile(t, filepath.Join(paths.root, fixture.agents[i].ID, "clock"), []byte("unsafe"), 0o600)
		startOutageAgent(t, paths, fixture, i)
	}
}

func assertOutageClockQuarantine(t *testing.T, paths switchFixturePaths, fixture *switchEndpointFixture, i int) {
	t.Helper()
	root := filepath.Join(paths.root, fixture.agents[i].ID)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(root, "clock"), []byte("unsafe"), 0o600)
	environment := map[string]string{"EPOCHALYPSE_OUTAGE_AGENT": root, "EPOCHALYPSE_OUTAGE_ADDRESS": strings.TrimPrefix(fixture.agents[i].Endpoint, "https://")}
	for attempt := range 2 {
		_, _, err := runSwitchExecutableResult(binary, []string{"-test.run=^TestSwitchOutageAgentProcess$"}, environment)
		if err == nil || !outageSnapshot(t, paths, fixture, i).UnsafeClock {
			t.Fatal("unsafe clock permits readiness or loses quarantine")
		}
		if attempt == 0 {
			writeOutageClock(t, paths, fixture, i, time.Now().Unix())
		}
	}
}

func assertOutageObserverCopies(t *testing.T, paths switchFixturePaths, fixture *switchEndpointFixture, unavailable int) {
	t.Helper()
	var shares [][]byte
	var ciphertext []byte
	for i := range 3 {
		if i == unavailable {
			continue
		}
		manifest, content := readOutagePublicCopy(t, paths, fixture, i)
		enrolled := fixture.agents[i]
		frame, err := switchwire.VerifyEnvelope(*manifest.Release, map[string]ed25519.PublicKey{enrolled.ID: ed25519.PublicKey(enrolled.SigningKey[:])})
		if err != nil || !frame.Released || len(frame.Entries) != 1 {
			t.Fatalf("observer rejects release: %v", err)
		}
		shares = append(shares, frame.Entries[0].Data)
		ciphertext = content
		stopSwitchAgentProcess(fixture.processes[i])
		assertOutageListenerStopped(t, strings.TrimPrefix(enrolled.Endpoint, "https://"))
	}
	content, err := switchcontent.Recover(ciphertext, shares)
	if err != nil || string(content) != "positive executable switch fixture" {
		t.Fatalf("retained shares lose recovery after endpoint loss: %v", err)
	}
}

func readOutagePublicCopy(t *testing.T, paths switchFixturePaths, fixture *switchEndpointFixture, i int) (switchwire.PublicManifest, []byte) {
	t.Helper()
	var bundle struct {
		Plan struct{ State switchwire.State }
	}
	if err := json.Unmarshal(readSwitchEvidence(t, paths.bundle), &bundle); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(fixture.servers[i].Certificate())
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	url := fixture.agents[i].Endpoint + "/agent/v1/public/switches/" + bundle.Plan.State.Switch
	manifestBytes := readOutagePublic(t, client, url)
	var manifest switchwire.PublicManifest
	if err := json.NewDecoder(bytes.NewReader(manifestBytes)).Decode(&manifest); err != nil || manifest.Release == nil {
		t.Fatalf("public observer lacks release: %v", err)
	}
	return manifest, readOutagePublic(t, client, url+"/ciphertext")
}

func readOutagePublic(t *testing.T, client *http.Client, url string) []byte {
	t.Helper()
	response, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, 11<<20))
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("public copy fails: status=%d error=%v", response.StatusCode, err)
	}
	return data
}
