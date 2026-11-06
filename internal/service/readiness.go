package service

import (
	"github.com/Crank-Git/epochalypse/internal/runtimeinfo"
	bolt "go.etcd.io/bbolt"
)

func (s *mainState) readiness(config Config) runtimeinfo.Report {
	report := runtimeinfo.New("main", "")
	report.Synthetic = config.Environment != "production" || config.LocalLab
	report.ConfigurationReady = config.Validate() == nil
	if s.owner != nil && s.content != nil {
		_, ownerErr := s.owner.Dashboard(1)
		contentErr := s.content.View(func(_ *bolt.Tx) error { return nil })
		report.StateReady = ownerErr == nil && contentErr == nil
	}
	// Main-service wall time has no verified error bound.
	report.Ready = report.StateReady && report.ConfigurationReady
	report.ProductionReady = report.Ready && !report.Synthetic
	return report
}
