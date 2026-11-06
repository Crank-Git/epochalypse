package deployment

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/runtimeinfo"
	"github.com/Crank-Git/epochalypse/internal/version"
)

func TestProbePinsIdentityAndWithholdsReadinessOnFailure(t *testing.T) {
	prior := version.Commit
	version.Commit = "0123456789abcdef0123456789abcdef01234567"
	defer func() { version.Commit = prior }()
	for _, mode := range []string{"healthy", "identity", "pin", "clock", "oversize", "redirect", "scaffold"} {
		t.Run(mode, func(t *testing.T) {
			inventory, roots := probeFixture(t, mode)
			report, err := Check(context.Background(), inventory, roots)
			if err != nil {
				t.Fatal(err)
			}
			if report.Ready != (mode == "healthy") || report.ProductionReady || report.PhysicalIndependenceVerified || len(report.Services) != 4 {
				t.Fatalf("%s: %+v", mode, report)
			}
		})
	}
}

func probeFixture(t *testing.T, mode string) (Inventory, *x509.CertPool) {
	t.Helper()
	inventory := inventoryFixture(t)
	roots := x509.NewCertPool()
	services := append([]Service{inventory.Main}, inventory.Agents...)
	for index := range services {
		item := &services[index]
		report := runtimeinfo.New("agent", item.ID)
		if index == 0 {
			report.Role = "main"
			report.AgentID = ""
		}
		report.Ready = true
		report.StateReady = true
		report.ClockReady = true
		report.ConfigurationReady = true
		report.Synthetic = true
		if mode == "identity" && index == 0 {
			report.Role = "agent"
		}
		if mode == "clock" && index == 0 {
			report.Ready = false
		}
		if mode == "scaffold" && index == 0 {
			report.Role = "scaffold"
		}
		server := httptest.NewTLSServer(metadataFixtureHandler(report, mode))
		t.Cleanup(server.Close)
		roots.AddCert(server.Certificate())
		pin := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
		item.Endpoint = server.URL
		item.TLSPin = hex.EncodeToString(pin[:])
	}
	inventory.Main = services[0]
	// Endpoint changes require a new canonical enrollment.
	inventory = resignEndpoints(t, inventory, services[1:])
	if mode == "pin" {
		inventory.Main.TLSPin = inventory.Main.TLSPin[:62] + "ff"
	}
	return inventory, roots
}

func metadataFixtureHandler(report runtimeinfo.Report, mode string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode == "redirect" {
			http.Redirect(w, r, "https://redirect.invalid", http.StatusFound)
			return
		}
		if mode == "oversize" {
			_, _ = w.Write(make([]byte, 4097))
			return
		}
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte("ok\n"))
			return
		}
		if !report.Ready && r.URL.Path == "/readyz" {
			w.WriteHeader(503)
		}
		_ = json.NewEncoder(w).Encode(report)
	})
}
