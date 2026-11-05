//go:build linux

package agent

import (
	"math"
	"testing"

	"golang.org/x/sys/unix"
)

func TestValidBootIDAcceptsHexCase(t *testing.T) {
	if !validBootID("ABCDEF01-2345-6789-abcd-EF0123456789") {
		t.Fatal("valid mixed-case boot ID was rejected")
	}
}

func TestValidBootIDRejectsInvalidUUIDParts(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "short", id: "00000000-0000-0000-0000-00000000000"},
		{name: "separator", id: "00000000_0000-0000-0000-000000000001"},
		{name: "non-hex", id: "00000000-0000-0000-0000-00000000000g"},
		{name: "space", id: "00000000-0000-0000-0000-00000000000 "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if validBootID(test.id) {
				t.Fatalf("boot ID %q was accepted", test.id)
			}
		})
	}
}

func TestDurationFromTimespecRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name  string
		value unix.Timespec
	}{
		{name: "negative seconds", value: unix.Timespec{Sec: -1}},
		{name: "negative nanoseconds", value: unix.Timespec{Nsec: -1}},
		{name: "nanoseconds at one second", value: unix.Timespec{Nsec: 1_000_000_000}},
		{name: "seconds exceed duration", value: unix.Timespec{Sec: math.MaxInt64/1_000_000_000 + 1}},
		{name: "nanoseconds overflow duration", value: unix.Timespec{Sec: math.MaxInt64 / 1_000_000_000, Nsec: 999_999_999}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := durationFromTimespec(test.value); err != ErrClock {
				t.Fatalf("durationFromTimespec(%+v) error = %v, want ErrClock", test.value, err)
			}
		})
	}
}

func TestSystemCaptureRejectsUnsupportedClock(t *testing.T) {
	if _, err := readLinuxClock(-1); err == nil {
		t.Fatal("unsupported clock ID was accepted")
	}
}
