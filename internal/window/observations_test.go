package window

import (
	"errors"
	"github.com/Crank-Git/epochalypse/internal/observation"
	"testing"
)

func observationTargets() []observation.Target {
	return []observation.Target{{ID: "strict", Address: "127.0.0.1:1053", Version: "1.22.0", VersionSource: "configured", Clock: observation.Clock{Source: "unknown"}}}
}

func TestObservationReservationSharesTheWindowQuota(t *testing.T) {
	s, err := OpenWithLimits(t.TempDir(), testSigner(t), Limits{MaxObjects: 1, MaxBytes: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	w, err := s.Create(validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveRun(w.ID, observationTargets(), testNow); !errors.Is(err, ErrQuota) {
		t.Fatalf("the observation bypasses the installation quota: %v", err)
	}
}

func TestObservationReservationPreservesTheWindowIdentity(t *testing.T) {
	s, err := Open(t.TempDir(), testSigner(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	w, err := s.Create(validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.ReserveRun(w.ID, observationTargets(), testNow)
	if err != nil || run.WindowID != w.ID || run.Name != w.Name || run.State != "pending" || len(run.Results) != 1 {
		t.Fatalf("the pending run loses its window: %+v: %v", run, err)
	}
}
