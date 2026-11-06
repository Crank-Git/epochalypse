package status

import (
	"context"
	"crypto/x509"

	"github.com/Crank-Git/epochalypse/internal/deployment"
)

// Result records complete inventory observations for one requested commit.
type Result struct {
	State      string            `json:"state"`
	Commit     string            `json:"commit"`
	Requested  string            `json:"requested"`
	Scope      string            `json:"scope"`
	Deployment deployment.Report `json:"deployment"`
}

// CheckInventory verifies the requested commit on every enrolled service.
func CheckInventory(ctx context.Context, inventory deployment.Inventory, requested string, production bool, roots *x509.CertPool) (Result, error) {
	result := Result{State: "unknown", Commit: "unknown", Requested: requested, Scope: "complete-inventory"}
	report, err := deployment.Check(ctx, inventory, roots)
	if err != nil {
		return result, err
	}
	result.Deployment = report
	result.State, result.Commit = summarize(report, requested, production)
	return result, nil
}

func summarize(report deployment.Report, requested string, production bool) (string, string) {
	commit, missing, wrong := observedCommits(report, requested)
	if wrong {
		return "failure", commit
	}
	for _, service := range report.Services {
		if service.Error != "" {
			return "unknown", commit
		}
	}
	if missing || len(report.Services) != 4 {
		return "unknown", commit
	}
	if !report.Ready || (production && !report.ProductionReady) {
		return "failure", commit
	}
	return "success", commit
}

func observedCommits(report deployment.Report, requested string) (string, bool, bool) {
	commit := ""
	missing, wrong := false, false
	for _, service := range report.Services {
		if !service.IdentityVerified || !ValidCommit(service.Metadata.Commit) {
			missing = true
			continue
		}
		observed := service.Metadata.Commit
		wrong = wrong || observed != requested
		if commit == "" {
			commit = observed
		} else if commit != observed {
			commit = "mixed"
		}
	}
	if missing || commit == "" {
		commit = "unknown"
	}
	return commit, missing, wrong
}
