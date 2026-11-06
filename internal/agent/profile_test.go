package agent

import (
	"testing"
	"time"
)

func validOperatorProfile() ClockProfile {
	return ClockProfile{Executable: "/usr/bin/chronyc", Version: "4.9", DaemonVersion: "4.9", OperatorVerified: true,
		Provenance: "operator pinned installation", Sources: []string{"192.0.2.1", "198.51.100.2"},
		ReferenceEvidence: "operator verified UTC sources", IndependenceEvidence: "operator verified independent references",
		LeapPolicy: "UTC normal only", BaseUncertainty: 100 * time.Millisecond, RatePPM: 100}
}

func TestProfileClockAcceptsExplicitOperatorPremises(t *testing.T) {
	clock, err := NewProfileClock(validOperatorProfile(), func() (Capture, error) { return Capture{}, nil })
	if err != nil || clock == nil {
		t.Fatalf("explicit profile clock=%v error=%v", clock, err)
	}
}

func TestProfileClockRejectsMissingOrUnverifiedPremises(t *testing.T) {
	for name, change := range map[string]func(*ClockProfile){
		"unverified":          func(p *ClockProfile) { p.OperatorVerified = false },
		"daemon":              func(p *ClockProfile) { p.DaemonVersion = "4.8" },
		"relative executable": func(p *ClockProfile) { p.Executable = "chronyc" },
		"reference":           func(p *ClockProfile) { p.ReferenceEvidence = "" },
		"independence":        func(p *ClockProfile) { p.IndependenceEvidence = "" },
		"rate":                func(p *ClockProfile) { p.RatePPM = 101 },
		"uncertainty":         func(p *ClockProfile) { p.BaseUncertainty = 501 * time.Millisecond },
		"leap":                func(p *ClockProfile) { p.LeapPolicy = "smear" },
	} {
		t.Run(name, func(t *testing.T) {
			p := validOperatorProfile()
			change(&p)
			if _, err := NewProfileClock(p, func() (Capture, error) { return Capture{}, nil }); err == nil {
				t.Fatal("profile accepts unverified premises")
			}
		})
	}
}
