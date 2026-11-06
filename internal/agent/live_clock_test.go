package agent

import (
	"errors"
	"testing"
	"time"
)

func TestChronyClockReassessesEachAcquisitionAfterFailure(t *testing.T) {
	fixture := chronyFixtureRunner()
	config := chronyTestConfig(fixture.run)
	assessment, _, _ := config.Evidence()
	config.Evidence = nil
	config.Profile = &assessment
	elapsed := time.Hour
	config.Capture = func() (Capture, error) {
		elapsed += time.Millisecond
		return Capture{Epoch: "new-process", UTCSeconds: time.Now().Unix(), CapturedElapsed: elapsed, DecisionElapsed: elapsed}, nil
	}
	clock, err := NewChronyClock(config)
	if err != nil {
		t.Fatal(err)
	}
	fixture.version = "invalid"
	if _, err := clock.Read(); err == nil {
		t.Fatal("clock accepts invalid command output")
	}
	fixture.version = "chronyc (chrony) version 4.9 (+READLINE)"
	if _, err := clock.Read(); err != nil {
		t.Fatalf("fresh assessment after failed acquisition: %v", err)
	}
}

func TestChronyClockRejectsChangedEpochBeforeFreshRead(t *testing.T) {
	fixture := chronyFixtureRunner()
	config := chronyTestConfig(fixture.run)
	assessment, _, _ := config.Evidence()
	config.Evidence = nil
	config.Profile = &assessment
	epoch := "first"
	config.Capture = func() (Capture, error) {
		capture := Capture{Epoch: epoch, UTCSeconds: time.Now().Unix(), CapturedElapsed: time.Hour, DecisionElapsed: time.Hour}
		epoch = "second"
		return capture, nil
	}
	clock, err := NewChronyClock(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clock.Read(); !errors.Is(err, errChronyEvidence) {
		t.Fatalf("epoch error=%v", err)
	}
	if _, err := clock.Read(); err != nil {
		t.Fatalf("fresh epoch remains unavailable: %v", err)
	}
}
