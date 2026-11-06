package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestChronyClockRejectsMalformedSafetyMetrics(t *testing.T) {
	for _, field := range []string{"System time", "Root delay", "Root dispersion", "Skew", "Leap status", "Reference ID"} {
		for _, value := range []string{"", "invalid", "NaN seconds", "-1 seconds", "1000 seconds"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				fixture := chronyFixtureRunner()
				lines := strings.Split(fixture.tracking, "\n")
				for i, line := range lines {
					if strings.HasPrefix(line, field+" ") {
						lines[i] = field + " : " + value
					}
				}
				fixture.tracking = strings.Join(lines, "\n")
				clock, err := NewChronyClock(chronyTestConfig(fixture.run))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := clock.Read(); err == nil {
					t.Fatal("malformed safety metric grants clock readiness")
				}
			})
		}
	}
}

func TestChronyClockRejectsStaleSourceUnits(t *testing.T) {
	for _, sample := range []string{"2m", "1h", "1d", "1y", "1z", "999999999999999999999999", "-1"} {
		t.Run(sample, func(t *testing.T) {
			fixture := chronyFixtureRunner()
			fixture.sources = strings.ReplaceAll(fixture.sources, "377 11", "377 "+sample)
			clock, err := NewChronyClock(chronyTestConfig(fixture.run))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := clock.Read(); err == nil {
				t.Fatal("stale source grants clock readiness")
			}
		})
	}
}

func TestChronyClockPreservesAcquisitionFailure(t *testing.T) {
	want := errors.New("synthetic acquisition failure")
	for _, operation := range []string{"tracking", "sources"} {
		t.Run(operation, func(t *testing.T) {
			fixture := chronyFixtureRunner()
			config := chronyTestConfig(func(ctx context.Context, executable string, args ...string) ([]byte, error) {
				if args[len(args)-1] == operation {
					return nil, want
				}
				return fixture.run(ctx, executable, args...)
			})
			clock, err := NewChronyClock(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := clock.Read(); !errors.Is(err, want) {
				t.Fatalf("acquisition error loses cause: %v", err)
			}
		})
	}
}

func TestChronyClockPreservesCaptureFailure(t *testing.T) {
	want := errors.New("synthetic capture failure")
	for _, failedCall := range []int{1, 2} {
		config := chronyTestConfig(chronyFixtureRunner().run)
		base := config.Evidence
		calls := 0
		config.Evidence = func() (Assessment, Capture, error) {
			calls++
			if calls == failedCall {
				return Assessment{}, Capture{}, want
			}
			return base()
		}
		clock, err := NewChronyClock(config)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := clock.Read(); !errors.Is(err, want) {
			t.Fatalf("capture error loses cause: %v", err)
		}
	}
}

func TestChronyClockRejectsInvalidOperatorPins(t *testing.T) {
	for name, change := range map[string]func(*ChronyConfig){
		"version":             func(c *ChronyConfig) { c.ExpectedVersion = "4.8" },
		"timeout short":       func(c *ChronyConfig) { c.Timeout = time.Nanosecond },
		"timeout long":        func(c *ChronyConfig) { c.Timeout = time.Minute },
		"one source":          func(c *ChronyConfig) { c.SourceIdentities = c.SourceIdentities[:1] },
		"duplicate sources":   func(c *ChronyConfig) { c.SourceIdentities[1] = c.SourceIdentities[0] },
		"empty source":        func(c *ChronyConfig) { c.SourceIdentities[1] = "" },
		"unpinned executable": func(c *ChronyConfig) { c.Executable = "" },
	} {
		t.Run(name, func(t *testing.T) {
			config := chronyTestConfig(chronyFixtureRunner().run)
			change(&config)
			if _, err := NewChronyClock(config); err == nil {
				t.Fatal("invalid operator pin grants clock readiness")
			}
		})
	}
}
