// Package runtimeinfo defines the public runtime metadata contract.
package runtimeinfo

import (
	"encoding/json"
	"net/http"

	"github.com/Crank-Git/epochalypse/internal/version"
)

// Report exposes readiness without private state or host evidence.
type Report struct {
	Version            int    `json:"version"`
	Commit             string `json:"commit"`
	Role               string `json:"role"`
	AgentID            string `json:"agent_id,omitempty"`
	StateFormat        int    `json:"state_format"`
	ProtocolFormat     int    `json:"protocol_format"`
	Ready              bool   `json:"ready"`
	ProductionReady    bool   `json:"production_ready"`
	StateReady         bool   `json:"state_ready"`
	ClockReady         bool   `json:"clock_ready"`
	ConfigurationReady bool   `json:"configuration_ready"`
	RecoveryAvailable  bool   `json:"recovery_available"`
	Synthetic          bool   `json:"synthetic"`
}

// New returns versioned metadata for one service identity.
func New(role, id string) Report {
	return Report{Version: 1, Commit: version.Commit, Role: role, AgentID: id, StateFormat: 1, ProtocolFormat: 2}
}

// Routes attaches liveness and read-only runtime metadata.
func Routes(mux *http.ServeMux, assess func() Report) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) { write(w, assess(), false) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { write(w, assess(), true) })
}

func write(w http.ResponseWriter, report Report, readiness bool) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if readiness && !report.Ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(report)
}
