// Package switches stores unarmed switch drafts.
package switches

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	DefaultIntervalSeconds int64 = 24 * 60 * 60
	DefaultGraceSeconds    int64 = 60 * 60
	MinIntervalSeconds     int64 = 60 * 60
	MaxIntervalSeconds     int64 = 365 * 24 * 60 * 60
	MaxGraceSeconds        int64 = 30 * 24 * 60 * 60
	StateDraft                   = "draft"
)

var (
	ErrInput       = errors.New("invalid switch input")
	ErrNotFound    = errors.New("switch does not exist")
	ErrIntegrity   = errors.New("invalid retained switch")
	ErrUnavailable = errors.New("switch creation is unavailable")
)

// Input contains the owner's configured durations.
type Input struct {
	IntervalSeconds *int64 `json:"interval_seconds"`
	GraceSeconds    *int64 `json:"grace_seconds"`
}

func (in Input) durations() (int64, int64, error) {
	interval := DefaultIntervalSeconds
	if in.IntervalSeconds != nil {
		interval = *in.IntervalSeconds
	}
	grace := DefaultGraceSeconds
	if in.GraceSeconds != nil {
		grace = *in.GraceSeconds
	}
	if interval < MinIntervalSeconds || interval > MaxIntervalSeconds || grace < 0 || grace > MaxGraceSeconds {
		return 0, 0, ErrInput
	}
	return interval, grace, nil
}

// Record contains versioned configuration without an authoritative deadline.
type Record struct {
	Version         int       `json:"version"`
	ID              string    `json:"id"`
	IntervalSeconds int64     `json:"interval_seconds"`
	GraceSeconds    int64     `json:"grace_seconds"`
	CreatedAt       time.Time `json:"created_at"`
	State           string    `json:"state"`
}

// ValidateRecord validates the stable persisted draft contract.
func ValidateRecord(record Record) error {
	if record.Version != 1 || !validID(record.ID) || record.State != StateDraft ||
		record.IntervalSeconds < MinIntervalSeconds || record.IntervalSeconds > MaxIntervalSeconds ||
		record.GraceSeconds < 0 || record.GraceSeconds > MaxGraceSeconds || record.CreatedAt.IsZero() ||
		record.CreatedAt.Location() != time.UTC || record.CreatedAt.Nanosecond() != 0 {
		return ErrIntegrity
	}
	return nil
}

func validID(id string) bool {
	_, err := hex.DecodeString(id)
	return len(id) == 32 && id == strings.ToLower(id) && err == nil
}
