package deployment

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/Crank-Git/epochalypse/internal/runtimeinfo"
	"github.com/Crank-Git/epochalypse/internal/switchclient"
	"io"
	"strings"
	"time"
)

// Observation records public measurements for one inventory identity.
type Observation struct {
	ID               string             `json:"id"`
	Healthy          bool               `json:"healthy"`
	IdentityVerified bool               `json:"identity_verified"`
	Metadata         runtimeinfo.Report `json:"metadata"`
	Error            string             `json:"error,omitempty"`
}

// Report separates measured runtime checks from operator declarations.
type Report struct {
	Version                      int           `json:"version"`
	Ready                        bool          `json:"ready"`
	ProductionReady              bool          `json:"production_ready"`
	PublicRecoveryVerified       bool          `json:"public_recovery_verified"`
	PhysicalIndependenceVerified bool          `json:"physical_independence_verified"`
	DeclarationIssues            []DomainIssue `json:"declaration_issues"`
	Services                     []Observation `json:"services"`
}

// Check measures every inventory endpoint with bounded pinned HTTPS requests.
func Check(ctx context.Context, inventory Inventory, roots *x509.CertPool) (Report, error) {
	if _, err := inventory.Validate(); err != nil {
		return Report{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	source := &switchclient.HTTPSReplica{Roots: roots, Pins: map[string][32]byte{}}
	services := append([]Service{inventory.Main}, inventory.Agents...)
	for _, s := range services {
		pin, _ := hex.DecodeString(s.TLSPin)
		source.Pins[s.Endpoint] = [32]byte(pin)
	}
	report := Report{Version: 1, Ready: true, ProductionReady: true, DeclarationIssues: inventory.DomainIssues(), Services: []Observation{}}
	for _, issue := range report.DeclarationIssues {
		if issue.Domain == "failure_domain" {
			report.Ready = false
			report.ProductionReady = false
		}
	}
	for index, s := range services {
		role, id := "agent", s.ID
		if index == 0 {
			role = "main"
			id = ""
		}
		observed := inspect(ctx, source, s, role, id)
		report.Services = append(report.Services, observed)
		report.Ready = report.Ready && observed.Healthy && observed.IdentityVerified && observed.Metadata.Ready
		report.ProductionReady = report.ProductionReady && observed.IdentityVerified && observed.Healthy && observed.Metadata.ProductionReady && !observed.Metadata.Synthetic
	}
	report.ProductionReady = report.ProductionReady && report.Ready
	return report, nil
}

func inspect(ctx context.Context, source *switchclient.HTTPSReplica, s Service, role, id string) Observation {
	observed := Observation{ID: s.ID}
	data, code, err := source.Metadata(ctx, s.Endpoint, "/version")
	if err != nil || code != 200 {
		observed.Error = "version unavailable"
		return observed
	}
	version, err := decodeMetadata(data, role, id)
	if err != nil {
		observed.Error = "runtime identity or format mismatch"
		return observed
	}
	observed.IdentityVerified = true
	observed.Metadata = version
	data, code, err = source.Metadata(ctx, s.Endpoint, "/healthz")
	observed.Healthy = err == nil && code == 200 && string(data) == "ok\n"
	if !observed.Healthy {
		observed.Error = "liveness unavailable"
		return observed
	}
	data, code, err = source.Metadata(ctx, s.Endpoint, "/readyz")
	if err != nil || (code != 200 && code != 503) {
		observed.Error = "readiness unavailable"
		observed.Metadata.Ready = false
		return observed
	}
	readiness, err := decodeMetadata(data, role, id)
	if err != nil || readiness.Commit != version.Commit || (code == 200) != readiness.Ready {
		observed.Error = "inconsistent readiness metadata"
		observed.Metadata.Ready = false
		return observed
	}
	observed.Metadata = readiness
	return observed
}

func decodeMetadata(data []byte, role, id string) (runtimeinfo.Report, error) {
	var report runtimeinfo.Report
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&report) != nil || decoder.Decode(new(any)) != io.EOF {
		return report, errors.New("invalid runtime metadata")
	}
	_, commitErr := hex.DecodeString(report.Commit)
	if report.Version != 1 || report.StateFormat != 1 || report.ProtocolFormat != 2 || report.Role != role || report.AgentID != id || commitErr != nil || len(report.Commit) != 40 || report.Commit != strings.ToLower(report.Commit) {
		return runtimeinfo.Report{}, errors.New("runtime identity mismatch")
	}
	return report, nil
}
