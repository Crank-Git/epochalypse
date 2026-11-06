package service

import (
	"encoding/json"
	"github.com/Crank-Git/epochalypse/internal/runtimeinfo"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMainProductionReadinessRequiresValidConfigurationAndOpenStores(t *testing.T) {
	for _, name := range []string{"production", "lab", "unspecified environment", "local lab", "invalid configuration", "uninitialized owner", "closed owner", "closed content"} {
		t.Run(name, func(t *testing.T) {
			config := Config{Mode: "server", StateDir: t.TempDir(), HTTPAddr: "127.0.0.1:8080", Environment: "production"}
			state, err := openMainState(config)
			if err != nil {
				t.Fatal(err)
			}
			defer state.close()
			if name != "uninitialized owner" {
				if err := state.owner.Initialize([]byte("synthetic readiness")); err != nil {
					t.Fatal(err)
				}
			}
			changeMainReadinessPrerequisite(t, name, &config, state)
			mux := http.NewServeMux()
			runtimeinfo.Routes(mux, func() runtimeinfo.Report { return state.readiness(config) })
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
			var report runtimeinfo.Report
			if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			wantReady := name == "production" || name == "lab" || name == "unspecified environment" || name == "local lab"
			wantStatus := http.StatusServiceUnavailable
			if wantReady {
				wantStatus = http.StatusOK
			}
			if response.Code != wantStatus || report.Ready != wantReady || report.ProductionReady != (name == "production") || report.ClockReady {
				t.Fatalf("main readiness: status=%d report=%+v", response.Code, report)
			}
		})
	}
}

func changeMainReadinessPrerequisite(t *testing.T, name string, config *Config, state *mainState) {
	t.Helper()
	switch name {
	case "lab":
		config.Environment = "lab"
	case "unspecified environment":
		config.Environment = ""
	case "local lab":
		config.LocalLab = true
	case "invalid configuration":
		config.HTTPAddr = ""
	case "closed owner":
		if err := state.owner.Close(); err != nil {
			t.Fatal(err)
		}
	case "closed content":
		if err := state.content.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScaffoldNeverReportsRuntimeReadiness(t *testing.T) {
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
	var report runtimeinfo.Report
	if response.Code != 503 || json.Unmarshal(response.Body.Bytes(), &report) != nil || report.Ready || report.ProductionReady {
		t.Fatalf("scaffold readiness: %d %s", response.Code, response.Body)
	}
}

func TestMainReadinessChecksOpenStoresWithoutAgentClockProfile(t *testing.T) {
	config := Config{Mode: "server", StateDir: t.TempDir(), HTTPAddr: "127.0.0.1:8080", Environment: "lab"}
	state, err := openMainState(config)
	if err != nil {
		t.Fatal(err)
	}
	defer state.close()
	if report := state.readiness(config); report.Ready {
		t.Fatal("uninitialized owner passes readiness")
	}
	if err := state.owner.Initialize([]byte("synthetic readiness")); err != nil {
		t.Fatal(err)
	}
	report := state.readiness(config)
	if !report.Ready || report.ProductionReady || report.ClockReady || !report.Synthetic {
		t.Fatalf("main readiness: %+v", report)
	}
	if err := state.content.Close(); err != nil {
		t.Fatal(err)
	}
	if state.readiness(config).Ready {
		t.Fatal("closed store passes readiness")
	}
}
