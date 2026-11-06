package command

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/deployment"
	"github.com/Crank-Git/epochalypse/internal/runtimeinfo"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestStatusChecksEveryRequiredService(t *testing.T) {
	for _, tc := range []struct{ mode, state, commit string }{
		{"ready", "success", strings.Repeat("a", 40)},
		{"wrong", "failure", strings.Repeat("b", 40)},
		{"mixed", "failure", "mixed"},
		{"missing", "unknown", "unknown"},
		{"identity", "unknown", "unknown"},
		{"format", "unknown", "unknown"},
		{"pin", "unknown", "unknown"},
		{"readiness", "failure", strings.Repeat("a", 40)},
		{"unavailable", "unknown", strings.Repeat("a", 40)},
		{"oversize", "unknown", "unknown"},
		{"redirect", "unknown", "unknown"},
		{"synthetic", "failure", strings.Repeat("a", 40)},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			path, _ := statusInventoryFixture(t, tc.mode)
			var out, diagnostic bytes.Buffer
			code := Run([]string{"status", "--inventory", path, "--production", strings.Repeat("a", 40)}, &out, &diagnostic)
			want := tc.state + " local " + tc.commit + "\n"
			if code != 0 || out.String() != want {
				t.Fatalf("exit=%d output=%q want=%q diagnostic=%s", code, &out, want, &diagnostic)
			}
		})
	}
}

func TestStatusRejectsIncompleteInventoryAndConflictingArguments(t *testing.T) {
	path, inventory := statusInventoryFixture(t, "ready")
	inventory.Agents = inventory.Agents[:2]
	writeJSONFixture(t, path, inventory)
	for _, flags := range [][]string{{"--inventory", path}, {"--inventory", path + ".missing"}, {"--production"}, {"--json"}, {"--inventory", path, "--endpoints", "https://example.invalid"}} {
		var out, diagnostic bytes.Buffer
		code := Run(append(append([]string{"status"}, flags...), strings.Repeat("a", 40)), &out, &diagnostic)
		if flags[0] == "--inventory" && len(flags) == 2 {
			if code != 0 || out.String() != "unknown local unknown\n" || diagnostic.Len() == 0 {
				t.Fatalf("inventory rejection: %d %s %s", code, &out, &diagnostic)
			}
		} else if code != 2 || out.Len() != 0 {
			t.Fatalf("argument rejection: %d %s %s", code, &out, &diagnostic)
		}
	}
}

func TestStatusJSONReportsCompleteScopeAndMeasuredServices(t *testing.T) {
	path, _ := statusInventoryFixture(t, "ready")
	var out, diagnostic bytes.Buffer
	if code := Run([]string{"status", "--inventory", path, "--json", strings.Repeat("a", 40)}, &out, &diagnostic); code != 0 {
		t.Fatalf("status: %d %s", code, &diagnostic)
	}
	var report struct {
		State      string            `json:"state"`
		Scope      string            `json:"scope"`
		Deployment deployment.Report `json:"deployment"`
	}
	if json.Unmarshal(out.Bytes(), &report) != nil || report.State != "success" || report.Scope != "complete-inventory" || len(report.Deployment.Services) != 4 || report.Deployment.PhysicalIndependenceVerified {
		t.Fatalf("status report: %s", &out)
	}
	if code := Run([]string{"status", "--inventory", path, "--json", strings.Repeat("a", 40)}, failedWriter{}, &diagnostic); code != 1 {
		t.Fatalf("write failure: %d", code)
	}
}

func statusInventoryFixture(t *testing.T, mode string) (string, deployment.Inventory) {
	t.Helper()
	fixture := newSwitchTLSServers(t)
	describeSwitchAgents(t, fixture)
	main := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(main.Close)
	owner, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	certificate := switchwire.Certificate{Version: 2, Interval: 3600, Grace: 60, Key: [32]byte{1}, Content: [32]byte{2}, Agents: fixture.agents}
	copy(certificate.OwnerKey[:], owner)
	encoded, err := switchwire.EncodeCertificate(certificate)
	if err != nil {
		t.Fatal(err)
	}
	inventory := deployment.Inventory{Version: 1, OwnerKey: owner, Certificate: encoded, CertificateSignature: ed25519.Sign(private, encoded)}
	servers := append([]*httptest.Server{main}, fixture.servers...)
	var roots []byte
	for index, server := range servers {
		id, role := "main", "main"
		if index > 0 {
			id, role = fixture.agents[index-1].ID, "agent"
		}
		metadata := runtimeinfo.Report{Version: 1, Commit: strings.Repeat("a", 40), Role: role, StateFormat: 1, ProtocolFormat: 2, Ready: true, ProductionReady: true, StateReady: true, ConfigurationReady: true}
		if index > 0 {
			metadata.AgentID = id
			metadata.ClockReady = true
		}
		metadata = statusFixtureMetadata(metadata, mode, index)
		server.Config.Handler = statusMetadataHandler(metadata, mode, index == 3)
		pin := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
		service := deployment.Service{ID: id, Endpoint: server.URL, TLSPin: hex.EncodeToString(pin[:]), FailureDomain: id, Evidence: "isolated synthetic fixture"}
		if mode == "pin" && index == 3 {
			service.TLSPin = strings.Repeat("01", 32)
		}
		if index == 0 {
			inventory.Main = service
		} else {
			inventory.Agents = append(inventory.Agents, service)
		}
		roots = append(roots, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})...)
	}
	root := t.TempDir()
	trust := filepath.Join(root, "roots.pem")
	writeFixtureFile(t, trust, roots, 0600)
	t.Setenv("SSL_CERT_FILE", trust)
	path := filepath.Join(root, "inventory.json")
	writeJSONFixture(t, path, inventory)
	return path, inventory
}

func statusFixtureMetadata(metadata runtimeinfo.Report, mode string, index int) runtimeinfo.Report {
	if mode == "wrong" || (mode == "mixed" && index == 3) {
		metadata.Commit = strings.Repeat("b", 40)
	}
	if index != 3 {
		return metadata
	}
	switch mode {
	case "missing":
		metadata.Commit = ""
	case "identity":
		metadata.AgentID = "other-agent"
	case "format":
		metadata.StateFormat = 3
	case "readiness":
		metadata.Ready = false
		metadata.ProductionReady = false
	case "synthetic":
		metadata.Synthetic = true
		metadata.ProductionReady = false
	}
	return metadata
}

func statusMetadataHandler(metadata runtimeinfo.Report, mode string, last bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if last && mode == "redirect" {
			http.Redirect(w, r, "https://example.invalid", http.StatusFound)
			return
		}
		if last && mode == "oversize" {
			_, _ = w.Write(make([]byte, 4097))
			return
		}
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte("ok\n"))
			return
		}
		if r.URL.Path == "/readyz" {
			if last && mode == "unavailable" {
				w.WriteHeader(500)
				return
			}
			if !metadata.Ready {
				w.WriteHeader(503)
			}
		}
		_ = json.NewEncoder(w).Encode(metadata)
	})
}

func TestCLIStatusChecksPinnedInventory(t *testing.T) {
	binary := os.Getenv("EPOCHALYPSE_SWITCH_ARM_BINARY")
	if binary == "" {
		return
	}
	for _, mode := range []string{"ready", "mixed", "pin"} {
		t.Run(mode, func(t *testing.T) {
			path, _ := statusInventoryFixture(t, mode)
			output := runSwitchExecutable(t, binary, []string{"status", "--inventory", path, "--production", strings.Repeat("a", 40)}, map[string]string{"SSL_CERT_FILE": os.Getenv("SSL_CERT_FILE")})
			want := map[string]string{"ready": "success local " + strings.Repeat("a", 40), "mixed": "failure local mixed", "pin": "unknown local unknown"}[mode]
			if strings.TrimSpace(string(output)) != want {
				t.Fatalf("status: %s", output)
			}
		})
	}
}
