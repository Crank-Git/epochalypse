//go:build linux

package agent

import (
	"math"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSystemCaptureReadsLinuxClockSources(t *testing.T) {
	read, err := NewSystemCapture()
	if err != nil {
		t.Fatal(err)
	}
	capture, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if capture.Epoch == "" || capture.UTCSeconds < 1 || capture.DecisionElapsed < capture.CapturedElapsed ||
		capture.DecisionElapsed-capture.CapturedElapsed > 250*time.Millisecond {
		t.Fatalf("capture = %+v", capture)
	}
}

func TestDurationFromTimespecRejectsOverflow(t *testing.T) {
	if _, err := durationFromTimespec(unix.Timespec{Sec: math.MaxInt64}); err == nil {
		t.Fatal("timespec overflow was accepted")
	}
}

func TestValidBootIDRequiresUUIDSyntax(t *testing.T) {
	if !validBootID("00000000-0000-0000-0000-000000000001") {
		t.Fatal("valid boot ID was rejected")
	}
	if validBootID("00000000-0000-0000-0000-00000000000xz1") {
		t.Fatal("malformed boot ID was accepted")
	}
}
