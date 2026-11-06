package command

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/agent"
	"github.com/Crank-Git/epochalypse/internal/version"
)

// TestSwitchOutageAgentProcess supplies clock control only in the test executable.
func TestSwitchOutageAgentProcess(t *testing.T) {
	root := os.Getenv("EPOCHALYPSE_OUTAGE_AGENT")
	if root == "" {
		return
	}
	if version.Commit != os.Getenv("EPOCHALYPSE_OUTAGE_COMMIT") {
		t.Fatal("agent test executable differs from the deployed commit")
	}
	data, err := os.ReadFile(filepath.Join(root, "runtime.json")) // #nosec G304 G703 -- The parent supplies this isolated test configuration.
	if err != nil {
		t.Fatal(err)
	}
	var settings agentRuntimeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	clock := func() (agent.Reading, error) {
		data, err := os.ReadFile(filepath.Join(root, "clock")) // #nosec G304 G703 -- The child reads its protected test clock.
		if err != nil {
			return agent.Reading{}, err
		}
		var sample outageClock
		if err := json.Unmarshal(data, &sample); err != nil {
			return agent.Reading{}, agent.ErrClock
		}
		elapsed := time.Since(time.Unix(0, sample.RecordedNano))
		if elapsed < 0 {
			return agent.Reading{}, agent.ErrClock
		}
		return agent.NewLabClock(func() time.Time { return time.Unix(sample.UTCSeconds, 0).Add(elapsed) })()
	}
	runtime, err := agent.OpenService(agent.ServiceConfig{Authority: settings.Authority, LabOnlySyntheticClock: true,
		Transition: agent.TransitionConfig{OwnerKey: settings.OwnerKey, EnrollmentBody: settings.Certificate,
			EnrollmentSignature: settings.CertificateSignature, AgentID: settings.AgentID,
			SigningKey: settings.SigningKey, AgeIdentity: settings.AgeIdentity, ReadClock: clock}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	certificate, err := tls.LoadX509KeyPair(filepath.Join(root, "tls.crt"), filepath.Join(root, "tls.key"))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", os.Getenv("EPOCHALYPSE_OUTAGE_ADDRESS"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/checkins") {
			if _, err := os.Stat(filepath.Join(root, "lose-receipts")); err == nil { // #nosec G703 -- The parent controls receipt loss in this isolated test directory.
				recorded := httptest.NewRecorder()
				runtime.Handler().ServeHTTP(recorded, r)
				if recorded.Code != http.StatusOK {
					t.Errorf("request rejected before receipt loss: %d", recorded.Code)
				}
				http.Error(w, "synthetic receipt loss after acceptance", http.StatusServiceUnavailable)
				return
			}
		}
		runtime.Handler().ServeHTTP(w, r)
	})
	server := &http.Server{Handler: handler, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}},
		ReadHeaderTimeout: time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: time.Second}
	defer func() { _ = server.Close() }()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	result := make(chan error, 2)
	go func() { result <- server.ServeTLS(listener, "", "") }()
	go func() { result <- runtime.RunPublisher(ctx) }()
	select {
	case <-ctx.Done():
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
}
