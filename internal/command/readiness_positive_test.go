package command

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/deployment"
	"github.com/Crank-Git/epochalypse/internal/service"
	"github.com/Crank-Git/epochalypse/internal/switchclient"
)

func verifyReadinessPublicRecovery(t *testing.T, binary string, paths switchFixturePaths, plan switchclient.Plan, fixture *switchEndpointFixture, owner ed25519.PublicKey) {
	t.Helper()
	main := httptest.NewTLSServer(service.Handler())
	defer main.Close()
	roots, err := os.ReadFile(paths.certs)
	if err != nil {
		t.Fatal(err)
	}
	roots = append(roots, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: main.Certificate().Raw})...)
	writeFixtureFile(t, paths.certs, roots, 0600)
	pin := sha256.Sum256(main.Certificate().RawSubjectPublicKeyInfo)
	inventory := deployment.Inventory{Version: 1, OwnerKey: owner, Certificate: plan.Certificate, CertificateSignature: plan.CertificateSignature, Main: deployment.Service{ID: "main", Endpoint: main.URL, TLSPin: hex.EncodeToString(pin[:])}}
	for index, a := range fixture.agents {
		pin := sha256.Sum256(fixture.servers[index].Certificate().RawSubjectPublicKeyInfo)
		inventory.Agents = append(inventory.Agents, deployment.Service{ID: a.ID, Endpoint: a.Endpoint, TLSPin: hex.EncodeToString(pin[:]), FailureDomain: a.ID, Evidence: "synthetic isolated fixture"})
	}
	path := filepath.Join(paths.root, "deployment.json")
	writeJSONFixture(t, path, inventory)
	output, stderr, err := runSwitchExecutableResult(binary, []string{"readiness", "--inventory", path, "--recovery-bundle", paths.bundle}, map[string]string{"SSL_CERT_FILE": paths.certs})
	var report deployment.Report
	if err == nil || json.Unmarshal(output.Bytes(), &report) != nil || !report.PublicRecoveryVerified || report.Ready || report.ProductionReady || strings.Contains(output.String(), "positive executable switch fixture") {
		t.Fatalf("readiness recovery: %s %s %v", output, stderr, err)
	}
}
