package capsule

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const metadataAllowance = 8192

func prepareInput(input Input, now time.Time) (Input, error) {
	if input.Mode != Public && input.Mode != ServerHeld {
		return Input{}, ErrUnsupportedMode
	}
	if input.Kind != Text && input.Kind != File {
		return Input{}, ErrInput
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Filename == "" {
		input.Filename = "capsule.bin"
		if input.Kind == Text {
			input.Filename = "capsule.txt"
		}
	}
	if !validLabel(input.Title, 256) || !validLabel(input.Filename, 255) || strings.ContainsAny(input.Filename, `/\`) {
		return Input{}, ErrInput
	}
	input.ReleaseAt = input.ReleaseAt.UTC()
	if !validStoredTime(now) || !validStoredTime(input.ReleaseAt) || input.ReleaseAt.Before(now) {
		return Input{}, ErrInput
	}
	return input, nil
}

func validLabel(value string, limit int) bool {
	if len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validStoredTime(value time.Time) bool {
	_, offset := value.Zone()
	return !value.IsZero() && value.Year() >= 1 && value.Year() <= 9999 && value.Nanosecond() == 0 && offset == 0
}

func validateManifest(m Manifest) error {
	input := Input{Mode: m.Mode, Kind: m.Kind, Title: m.Title, Filename: m.Filename, ReleaseAt: m.RequestedReleaseAt}
	normalized, err := prepareInput(input, m.CreatedAt)
	if err != nil || m.Version != FormatVersion || m.PlaintextBytes < 0 || normalized.Title != m.Title || normalized.Filename != m.Filename || !validStoredTime(m.RequestedReleaseAt) {
		return ErrIntegrity
	}
	if m.PublishedAt != nil && (!validStoredTime(*m.PublishedAt) || m.PublishedAt.Before(m.CreatedAt)) {
		return ErrIntegrity
	}
	if m.Mode == Public {
		return validatePublicTime(m)
	}
	if m.EffectiveReleaseAt == nil || !m.EffectiveReleaseAt.Equal(m.RequestedReleaseAt) || !validStoredTime(*m.EffectiveReleaseAt) {
		return ErrIntegrity
	}
	return nil
}

func validatePublicTime(m Manifest) error {
	if m.PublishedAt == nil {
		if m.EffectiveReleaseAt != nil {
			return ErrIntegrity
		}
		return nil
	}
	if m.EffectiveReleaseAt == nil || !m.EffectiveReleaseAt.Equal(*m.PublishedAt) || !validStoredTime(*m.EffectiveReleaseAt) {
		return ErrIntegrity
	}
	return nil
}
