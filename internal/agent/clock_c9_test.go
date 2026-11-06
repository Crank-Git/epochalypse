package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLabClockRejectsMissingTimeSource(t *testing.T) {
	if _, err := NewLabClock(nil)(); !errors.Is(err, ErrClock) {
		t.Fatalf("missing time source grants readiness: %v", err)
	}
}

func TestChronyClockRejectsUninitializedReader(t *testing.T) {
	for _, clock := range []*ChronyClock{nil, {}} {
		if reading, err := clock.Read(); !errors.Is(err, errChronyEvidence) || reading.LowerUTC() != 0 {
			t.Fatalf("uninitialized reader grants readiness: %+v, %v", reading, err)
		}
	}
}

func TestChronyClockRejectsCaptureFromAnotherAssessmentEpoch(t *testing.T) {
	config := chronyTestConfig(chronyFixtureRunner().run)
	base := config.Evidence
	config.Evidence = func() (Assessment, Capture, error) {
		assessment, capture, err := base()
		capture.Epoch = "unassessed-clock-epoch"
		return assessment, capture, err
	}
	clock, err := NewChronyClock(config)
	if err != nil {
		t.Fatal(err)
	}
	if reading, err := clock.Read(); !errors.Is(err, ErrClock) || reading.LowerUTC() != 0 {
		t.Fatalf("unassessed capture grants readiness: %+v, %v", reading, err)
	}
}

func TestChronyClockRejectsOversizedAcquisition(t *testing.T) {
	fixture := chronyFixtureRunner()
	config := chronyTestConfig(func(ctx context.Context, executable string, args ...string) ([]byte, error) {
		if args[len(args)-1] == "tracking" {
			return []byte(strings.Repeat("x", chronyOutputLimit+1)), nil
		}
		return fixture.run(ctx, executable, args...)
	})
	clock, err := NewChronyClock(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clock.Read(); !errors.Is(err, errChronyEvidence) {
		t.Fatalf("oversized output grants readiness: %v", err)
	}
}

func TestCaptureWithholdsReadinessUntilBaselineRecovers(t *testing.T) {
	fault := errors.New("clock acquisition unavailable")
	active := false
	config := systemCaptureConfig{
		readClock: advancingSystemClock(),
		readBootID: func() (string, error) {
			if active {
				return "", fault
			}
			return "boot-one", nil
		},
		wallNow: advancingSystemWall(), processID: 42, processEpoch: "process-one",
	}
	read, err := newSystemCapture(config)
	if err != nil {
		t.Fatal(err)
	}
	first, err := read()
	if err != nil {
		t.Fatal(err)
	}
	active = true
	for range 2 {
		capture, err := read()
		if !errors.Is(err, fault) || capture.Epoch != "" {
			t.Fatalf("failed baseline returns usable evidence: %+v, %v", capture, err)
		}
	}
	active = false
	got, err := read()
	if err != nil || got.Epoch == first.Epoch {
		t.Fatalf("baseline recovery retains old epoch: %+v, %v", got, err)
	}
}

func TestCaptureRejectsNegativeClockSampleAndReassesses(t *testing.T) {
	for _, source := range []int32{clockBoottime, clockMonotonic} {
		t.Run(map[int32]string{clockBoottime: "boottime", clockMonotonic: "monotonic"}[source], func(t *testing.T) {
			active := false
			clock := advancingSystemClock()
			read, err := newSystemCapture(systemCaptureConfig{
				readClock: func(id int32) (time.Duration, error) {
					if active && id == source {
						return -time.Second, nil
					}
					return clock(id)
				},
				readBootID: func() (string, error) { return "boot-one", nil },
				wallNow:    advancingSystemWall(), processID: 42, processEpoch: "process-one",
			})
			if err != nil {
				t.Fatal(err)
			}
			first, err := read()
			if err != nil {
				t.Fatal(err)
			}
			active = true
			if got, err := read(); !errors.Is(err, ErrClock) || got.Epoch != "" {
				t.Fatalf("negative clock sample grants readiness: %+v, %v", got, err)
			}
			active = false
			got, err := read()
			if err != nil || got.Epoch == first.Epoch {
				t.Fatalf("negative sample preserves old epoch: %+v, %v", got, err)
			}
		})
	}
}
