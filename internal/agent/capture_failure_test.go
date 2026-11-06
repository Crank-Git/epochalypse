package agent

import (
	"errors"
	"testing"
	"time"
)

func TestCaptureRejectsAcquisitionFailureUntilFreshBaseline(t *testing.T) {
	for _, source := range []string{"boot-id", "boottime", "monotonic"} {
		for _, phase := range []string{"before-wall", "after-wall"} {
			t.Run(source+"/"+phase, func(t *testing.T) {
				fault := errors.New("synthetic capture acquisition failure")
				active, afterWall := false, false
				read, err := newSystemCapture(captureFailureConfig(source, phase, fault, &active, &afterWall))
				if err != nil {
					t.Fatal(err)
				}
				first, err := read()
				if err != nil {
					t.Fatal(err)
				}
				active, afterWall = true, false
				if _, err := read(); !errors.Is(err, fault) || !errors.Is(err, ErrClock) {
					t.Fatalf("capture acquisition error loses cause: %v", err)
				}
				active = false
				recovered, err := read()
				if err != nil || recovered.Epoch == first.Epoch {
					t.Fatalf("capture failure retains old epoch: %v", err)
				}
			})
		}
	}
}

func TestCaptureRejectsUnavailableInitialBaseline(t *testing.T) {
	for _, source := range []string{"boot-id", "boottime", "monotonic"} {
		t.Run(source, func(t *testing.T) {
			fault := errors.New("synthetic baseline failure")
			active, afterWall := true, false
			_, err := newSystemCapture(captureFailureConfig(source, "before-wall", fault, &active, &afterWall))
			if !errors.Is(err, fault) || !errors.Is(err, ErrClock) {
				t.Fatalf("baseline failure grants readiness or loses cause: %v", err)
			}
		})
	}
}

func captureFailureConfig(source, phase string, fault error, active, afterWall *bool) systemCaptureConfig {
	shouldFail := func() bool { return *active && *afterWall == (phase == "after-wall") }
	clock, wall := advancingSystemClock(), advancingSystemWall()
	return systemCaptureConfig{
		readClock: func(id int32) (time.Duration, error) {
			if shouldFail() && ((source == "boottime" && id == clockBoottime) || (source == "monotonic" && id == clockMonotonic)) {
				return 0, fault
			}
			return clock(id)
		},
		readBootID: func() (string, error) {
			if shouldFail() && source == "boot-id" {
				return "", fault
			}
			return "boot-one", nil
		},
		wallNow:   func() time.Time { *afterWall = true; return wall() },
		processID: 42, processEpoch: "process-one",
	}
}
