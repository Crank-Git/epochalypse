// Package window stores public TXT records with explicit DNSSEC signature windows.
package window

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Input contains public content and explicit UTC boundaries.
type Input struct {
	Content    string `json:"content"`
	Inception  string `json:"inception"`
	Expiration string `json:"expiration"`
}

// ValidationError names the public field that requires correction.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Validate returns a ValidationError for invalid content or unsupported serial-time boundaries.
func (in Input) Validate(now time.Time) error {
	if !utf8.ValidString(in.Content) || len(in.Content) > 512 {
		return &ValidationError{"content", "content requires at most 512 UTF-8 bytes"}
	}
	inception, expiration, err := in.boundaries()
	if err != nil {
		return err
	}
	for _, boundary := range []struct {
		field string
		value int64
	}{{"inception", inception.Unix()}, {"expiration", expiration.Unix()}} {
		if distance := boundary.value - now.Unix(); distance <= -1<<31 || distance >= 1<<31 {
			return &ValidationError{boundary.field, "signature boundary exceeds the supported clock distance"}
		}
	}
	return nil
}

func (in Input) boundaries() (time.Time, time.Time, error) {
	inception, err := parseUTC(in.Inception)
	if err != nil {
		return time.Time{}, time.Time{}, &ValidationError{"inception", err.Error()}
	}
	expiration, err := parseUTC(in.Expiration)
	if err != nil {
		return time.Time{}, time.Time{}, &ValidationError{"expiration", err.Error()}
	}
	if duration := expiration.Unix() - inception.Unix(); duration <= 0 || duration >= 1<<31 {
		return time.Time{}, time.Time{}, &ValidationError{"expiration", "expiration must follow inception by less than 2147483648 seconds"}
	}
	return inception, expiration, nil
}

func parseUTC(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil || len(value) != 20 || !strings.HasSuffix(value, "Z") || t.Unix() < 0 || t.Unix() > math.MaxUint32 {
		return time.Time{}, errors.New("use whole UTC seconds from 1970-01-01T00:00:00Z through 2106-02-07T06:28:15Z")
	}
	return t, nil
}
