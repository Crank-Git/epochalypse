//go:build !linux

package agent

import (
	"errors"
	"testing"
)

func TestSystemCaptureRejectsUnsupportedPlatform(t *testing.T) {
	if _, err := NewSystemCapture(); !errors.Is(err, ErrClock) {
		t.Fatalf("constructor error = %v, want ErrClock", err)
	}
}
