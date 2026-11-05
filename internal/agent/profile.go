package agent

import (
	"path/filepath"
	"time"
)

// ClockProfile records explicit operator premises for the configured clock.
type ClockProfile struct {
	Executable           string        `json:"executable"`
	Version              string        `json:"version"`
	DaemonVersion        string        `json:"daemon_version"`
	OperatorVerified     bool          `json:"operator_verified"`
	Provenance           string        `json:"provenance"`
	Sources              []string      `json:"sources"`
	ReferenceEvidence    string        `json:"reference_evidence"`
	IndependenceEvidence string        `json:"independence_evidence"`
	LeapPolicy           string        `json:"leap_policy"`
	BaseUncertainty      time.Duration `json:"base_uncertainty_ns"`
	RatePPM              uint32        `json:"rate_ppm"`
}

// NewProfileClock requires explicit operator premises before clock acquisition.
func NewProfileClock(profile ClockProfile, capture func() (Capture, error)) (*ChronyClock, error) {
	if !profile.OperatorVerified || profile.Version != "4.9" || profile.DaemonVersion != "4.9" ||
		!filepath.IsAbs(profile.Executable) || profile.LeapPolicy != "UTC normal only" {
		return nil, errChronyEvidence
	}
	assessment := Assessment{Epoch: "profile-validation", Implementation: "chrony 4.9", Provenance: profile.Provenance,
		SourceIdentities: profile.Sources, ReferenceEvidence: profile.ReferenceEvidence, IndependenceEvidence: profile.IndependenceEvidence,
		LeapPolicy: profile.LeapPolicy, BaseUncertainty: profile.BaseUncertainty, RatePPM: profile.RatePPM}
	if !assessmentComplete(assessment) {
		return nil, errChronyEvidence
	}
	return NewChronyClock(ChronyConfig{Executable: profile.Executable, ExpectedVersion: profile.Version,
		SourceIdentities: profile.Sources, Profile: &assessment, Capture: capture})
}
