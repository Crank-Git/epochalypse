package service

import (
	"encoding/json"
	"github.com/Crank-Git/epochalypse/internal/runtimeinfo"
	"net/http/httptest"
	"testing"
)

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
