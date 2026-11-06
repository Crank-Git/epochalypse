//go:build linux

package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	clockBoottime  = int32(unix.CLOCK_BOOTTIME)
	clockMonotonic = int32(unix.CLOCK_MONOTONIC)
)

// NewSystemCapture returns read-only UTC and Linux elapsed-clock samples.
func NewSystemCapture() (func() (Capture, error), error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("create process clock epoch: %w", err)
	}
	return newSystemCapture(systemCaptureConfig{
		readClock:    readLinuxClock,
		readBootID:   readLinuxBootID,
		wallNow:      time.Now,
		processID:    os.Getpid(),
		processEpoch: hex.EncodeToString(nonce[:]),
	})
}

func readLinuxClock(clockID int32) (time.Duration, error) {
	var value unix.Timespec
	if err := unix.ClockGettime(clockID, &value); err != nil {
		return 0, err
	}
	return durationFromTimespec(value)
}

func durationFromTimespec(value unix.Timespec) (time.Duration, error) {
	if value.Sec < 0 || value.Nsec < 0 || value.Nsec >= 1_000_000_000 ||
		value.Sec > math.MaxInt64/int64(time.Second) {
		return 0, ErrClock
	}
	seconds := time.Duration(value.Sec) * time.Second
	if seconds > time.Duration(math.MaxInt64)-time.Duration(value.Nsec) {
		return 0, ErrClock
	}
	return seconds + time.Duration(value.Nsec), nil
}

func readLinuxBootID() (string, error) {
	file, err := os.Open("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	value, err := io.ReadAll(io.LimitReader(file, 129))
	if err != nil {
		return "", err
	}
	if len(value) > 128 {
		return "", ErrClock
	}
	bootID := strings.TrimSpace(string(value))
	if len(bootID) != 36 {
		return "", ErrClock
	}
	if !validBootID(bootID) {
		return "", ErrClock
	}
	return bootID, nil
}

func validBootID(bootID string) bool {
	if len(bootID) != 36 {
		return false
	}
	for index, char := range bootID {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}
	return true
}
