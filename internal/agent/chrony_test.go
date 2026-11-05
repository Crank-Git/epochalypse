package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestChronyClockAcceptsPinnedFreshEvidence(t *testing.T) {
	runner := chronyFixtureRunner()
	clock, err := NewChronyClock(chronyTestConfig(runner.run))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clock.Read(); err != nil {
		t.Fatal(err)
	}
}

func TestChronyClockRejectsUnpinnedVersion(t *testing.T) {
	runner := chronyFixtureRunner()
	runner.version = "chronyc (chrony) version 4.8"
	clock, err := NewChronyClock(chronyTestConfig(runner.run))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clock.Read(); err == nil {
		t.Fatal("clock accepted a chrony version outside the pin")
	}
}

func TestChronyClockAcceptsPinnedVersionWithFeatureSuffix(t *testing.T) {
	fixture := chronyFixtureRunner()
	fixture.version = "chronyc (chrony) version 4.9 (+READLINE +SECHASH +IPV6 -DEBUG)"
	clock, err := NewChronyClock(chronyTestConfig(fixture.run))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clock.Read(); err != nil {
		t.Fatal(err)
	}
}

func TestChronyClockRejectsUntrustedLeapAndStaleSource(t *testing.T) {
	for name, change := range map[string]func(*chronyFixture){
		"leap": func(f *chronyFixture) { f.tracking = strings.Replace(f.tracking, "Leap status     : Normal", "Leap status     : Not synchronised", 1) },
		"source": func(f *chronyFixture) { f.sources = strings.Replace(f.sources, "^+ 198.51.100.2", "^? 198.51.100.2", 1) },
		"stale": func(f *chronyFixture) { f.sources = strings.Replace(f.sources, "377 11", "000 1000", 1) },
		"no selected source": func(f *chronyFixture) { f.sources = strings.Replace(f.sources, "^* 192.0.2.1", "^+ 192.0.2.1", 1) },
		"wrong reference": func(f *chronyFixture) { f.tracking = strings.Replace(f.tracking, "(192.0.2.1)", "(203.0.113.8)", 1) },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := chronyFixtureRunner()
			change(fixture)
			clock, err := NewChronyClock(chronyTestConfig(fixture.run))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := clock.Read(); err == nil {
				t.Fatal("clock accepted invalid chrony evidence")
			}
		})
	}
}

func TestChronyClockCombinesIndependentAndChronyUncertainty(t *testing.T) {
	config := chronyTestConfig(chronyFixtureRunner().run)
	base := config.Evidence
	config.Evidence = func() (Assessment, Capture, error) {
		assessment, capture, err := base()
		assessment.BaseUncertainty = 499 * time.Millisecond
		return assessment, capture, err
	}
	clock, err := NewChronyClock(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clock.Read(); err == nil {
		t.Fatal("clock accepted combined uncertainty above the candidate ceiling")
	}
}

func TestChronyClockRejectsAnEpochChangeDuringRead(t *testing.T) {
	config := chronyTestConfig(chronyFixtureRunner().run)
	base := config.Evidence
	calls := 0
	config.Evidence = func() (Assessment, Capture, error) {
		assessment, capture, err := base()
		calls++
		if calls > 1 {
			assessment.Epoch = "new-epoch"
			capture.Epoch = "new-epoch"
		}
		return assessment, capture, err
	}
	clock, err := NewChronyClock(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clock.Read(); err == nil {
		t.Fatal("clock accepted a changed assessment epoch")
	}
}

func TestChronyClockRejectsCommandFailure(t *testing.T) {
	want := errors.New("chrony unavailable")
	config := chronyTestConfig(func(context.Context, string, ...string) ([]byte, error) {
		return nil, want
	})
	clock, err := NewChronyClock(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clock.Read(); !errors.Is(err, want) {
		t.Fatalf("clock error = %v, want %v", err, want)
	}
}

type chronyFixture struct {
	version, tracking, sources string
}

func chronyFixtureRunner() *chronyFixture {
	refTime := time.Now().UTC().Format("Mon Jan _2 15:04:05 2006")
	return &chronyFixture{
		version: "chronyc (chrony) version 4.9 (+READLINE +SECHASH +IPV6 -DEBUG)",
		tracking: "Reference ID    : CB00710F (192.0.2.1)\nRef time (UTC)  : " + refTime + "\nSystem time     : 0.000006523 seconds slow of NTP time\nSkew            : 0.129 ppm\nRoot delay      : 0.013639022 seconds\nRoot dispersion : 0.001100737 seconds\nLeap status     : Normal\n",
		sources: "MS Name/IP address         Stratum Poll Reach LastRx Last sample\n==============================================================================\n^* 192.0.2.1 2 6 377 11 -4us[ -6us] +/- 1ms\n^+ 198.51.100.2 2 6 377 11 -4us[ -6us] +/- 1ms\n",
	}
}

func (f *chronyFixture) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	switch args[len(args)-1] {
	case "-v":
		return []byte(f.version), nil
	case "tracking":
		return []byte(f.tracking), nil
	case "sources":
		return []byte(f.sources), nil
	default:
		return nil, errors.New("unexpected chronyc command")
	}
}

func chronyTestConfig(run ChronyRunner) ChronyConfig {
	return ChronyConfig{
		Executable: "chronyc", ExpectedVersion: "4.9", Run: run,
		SourceIdentities: []string{"192.0.2.1", "198.51.100.2"},
		Evidence: func() (Assessment, Capture, error) {
			now := time.Now()
			return Assessment{
				Epoch: "boot-epoch-1", Implementation: "chrony 4.9", Provenance: "operator profile",
				SourceIdentities: []string{"192.0.2.1", "198.51.100.2"},
				ReferenceEvidence: "operator verified source set", IndependenceEvidence: "operator verified independent sources",
				LeapPolicy: "UTC normal only", AssessedElapsed: 0, BaseUncertainty: 100 * time.Millisecond,
			}, Capture{Epoch: "boot-epoch-1", UTCSeconds: now.Unix(), CapturedElapsed: time.Second, DecisionElapsed: time.Second}, nil
		},
	}
}
