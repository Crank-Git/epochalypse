package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

const chronyOutputLimit = 64 << 10

// ChronyRunner executes one bounded chronyc command.
type ChronyRunner func(context.Context, string, ...string) ([]byte, error)

// ChronyConfig pins chrony and supplies independent clock evidence.
type ChronyConfig struct {
	Executable       string
	ExpectedVersion  string
	SourceIdentities []string
	Timeout          time.Duration
	Run              ChronyRunner
	Evidence         func() (Assessment, Capture, error)
	Profile          *Assessment
	Capture          func() (Capture, error)
}

// ChronyClock reads chrony state and applies operator-pinned clock evidence.
type ChronyClock struct{ config ChronyConfig }

// NewChronyClock creates a read-only adapter for chrony 4.9.
func NewChronyClock(config ChronyConfig) (*ChronyClock, error) {
	if config.Executable == "" || config.ExpectedVersion != "4.9" || !validEvidenceMode(config) || !validSources(config.SourceIdentities) {
		return nil, errChronyEvidence
	}
	if config.Timeout == 0 {
		config.Timeout = 2 * time.Second
	}
	if config.Timeout < time.Millisecond || config.Timeout > 5*time.Second {
		return nil, errChronyEvidence
	}
	if config.Run == nil {
		config.Run = runChronyCommand
	}
	config.SourceIdentities = slices.Clone(config.SourceIdentities)
	if config.Profile != nil {
		profile := *config.Profile
		profile.SourceIdentities = slices.Clone(profile.SourceIdentities)
		config.Profile = &profile
	}
	return &ChronyClock{config: config}, nil
}

func validEvidenceMode(config ChronyConfig) bool {
	if config.Profile != nil || config.Capture != nil {
		return config.Evidence == nil && config.Profile != nil && config.Capture != nil
	}
	return config.Evidence != nil
}

func validSources(sources []string) bool {
	if len(sources) < 2 || len(sources) > 8 {
		return false
	}
	seen := make(map[string]bool, len(sources))
	for _, source := range sources {
		if strings.TrimSpace(source) != source || source == "" || seen[source] {
			return false
		}
		seen[source] = true
	}
	return true
}

// Read returns one decision-local lower UTC bound.
func (c *ChronyClock) Read() (Reading, error) {
	if c == nil || !validEvidenceMode(c.config) || c.config.Run == nil {
		return Reading{}, errChronyEvidence
	}
	started := time.Now()
	initialAssessment, initialCapture, err := c.beginAssessment()
	if err != nil {
		return Reading{}, errors.Join(errChronyEvidence, err)
	}
	tracking, sources, err := c.readSnapshot()
	if err != nil {
		return Reading{}, err
	}
	assessment, capture, err := c.endAssessment(initialAssessment)
	if err != nil {
		return Reading{}, errors.Join(errChronyEvidence, err)
	}
	if !c.validWindow(started, initialAssessment, initialCapture, assessment, capture) {
		return Reading{}, errChronyEvidence
	}
	return evaluateChrony(string(tracking), string(sources), assessment, capture, c.config.SourceIdentities)
}

func (c *ChronyClock) beginAssessment() (Assessment, Capture, error) {
	if c.config.Profile == nil {
		return c.config.Evidence()
	}
	capture, err := c.config.Capture()
	assessment := *c.config.Profile
	assessment.Epoch = capture.Epoch
	assessment.AssessedElapsed = capture.CapturedElapsed
	return assessment, capture, err
}

func (c *ChronyClock) endAssessment(initial Assessment) (Assessment, Capture, error) {
	if c.config.Profile == nil {
		return c.config.Evidence()
	}
	capture, err := c.config.Capture()
	return initial, capture, err
}

func (c *ChronyClock) readSnapshot() ([]byte, []byte, error) {
	version, err := c.command("-v")
	if err != nil || !chronyVersionMatches(string(version), c.config.ExpectedVersion) {
		return nil, nil, errors.Join(errChronyEvidence, err)
	}
	tracking, err := c.command("-n", "tracking")
	if err != nil {
		return nil, nil, errors.Join(errChronyEvidence, err)
	}
	sources, err := c.command("-n", "sources")
	if err != nil {
		return nil, nil, errors.Join(errChronyEvidence, err)
	}
	return tracking, sources, nil
}

func (c *ChronyClock) validWindow(started time.Time, before Assessment, beforeCapture Capture, after Assessment, afterCapture Capture) bool {
	return time.Since(started) <= 250*time.Millisecond && sameAssessment(before, after) &&
		sameSources(after.SourceIdentities, c.config.SourceIdentities) && validCaptureWindow(beforeCapture, afterCapture)
}

func evaluateChrony(tracking, sources string, assessment Assessment, capture Capture, expected []string) (Reading, error) {
	metrics, err := parseChronyTracking(string(tracking))
	if err != nil || metrics.leap != "Normal" || metrics.skewPPM > 100 ||
		assessment.BaseUncertainty < 0 || metrics.errorBound > 500*time.Millisecond-assessment.BaseUncertainty {
		return Reading{}, errors.Join(errChronyEvidence, err)
	}
	if !selectedSources(sources, expected, metrics.referenceID) || !freshReference(metrics.referenceTime, capture.UTCSeconds) {
		return Reading{}, errChronyEvidence
	}
	reading, err := AssessClock(assessment, capture)
	if err != nil {
		return Reading{}, errors.Join(errChronyEvidence, err)
	}
	return reading, nil
}

func validCaptureWindow(before, after Capture) bool {
	if before.Epoch != after.Epoch || before.CapturedElapsed < 0 || after.CapturedElapsed < before.DecisionElapsed || before.DecisionElapsed < before.CapturedElapsed || after.DecisionElapsed < after.CapturedElapsed {
		return false
	}
	return after.DecisionElapsed-before.CapturedElapsed <= 250*time.Millisecond
}

func sameAssessment(before, after Assessment) bool {
	return before.Epoch == after.Epoch && before.Implementation == after.Implementation && before.Provenance == after.Provenance &&
		slices.Equal(before.SourceIdentities, after.SourceIdentities) && before.ReferenceEvidence == after.ReferenceEvidence &&
		before.IndependenceEvidence == after.IndependenceEvidence && before.LeapPolicy == after.LeapPolicy &&
		before.AssessedElapsed == after.AssessedElapsed && before.BaseUncertainty == after.BaseUncertainty && before.RatePPM == after.RatePPM
}

func chronyVersionMatches(output, expected string) bool {
	output = strings.TrimSpace(output)
	prefix := "chronyc (chrony) version " + expected
	if !strings.HasPrefix(output, prefix) {
		return false
	}
	suffix := strings.TrimSpace(strings.TrimPrefix(output, prefix))
	return len(suffix) >= 2 && strings.HasPrefix(suffix, "(") && strings.HasSuffix(suffix, ")") && !strings.Contains(suffix, "\n")
}

func (c *ChronyClock) command(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()
	result, err := c.config.Run(ctx, c.config.Executable, args...)
	if err != nil {
		return nil, fmt.Errorf("run chronyc: %w", err)
	}
	if len(result) > chronyOutputLimit {
		return nil, errChronyEvidence
	}
	return result, nil
}

type chronyMetrics struct {
	errorBound    time.Duration
	skewPPM       float64
	leap          string
	referenceTime time.Time
	referenceID   string
}

func parseChronyTracking(output string) (chronyMetrics, error) {
	fields, err := parseChronyFields(output)
	if err != nil {
		return chronyMetrics{}, err
	}
	return parseChronyMetrics(fields)
}

func parseChronyFields(output string) (map[string]string, error) {
	fields := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			if _, exists := fields[key]; exists {
				return nil, errChronyEvidence
			}
			fields[key] = strings.TrimSpace(parts[1])
		}
	}
	return fields, nil
}

func parseChronyMetrics(fields map[string]string) (chronyMetrics, error) {
	ref, err := time.Parse("Mon Jan _2 15:04:05 2006", fields["Ref time (UTC)"])
	if err != nil {
		return chronyMetrics{}, errChronyEvidence
	}
	offset, err := chronyValue(fields["System time"], " seconds")
	if err != nil {
		return chronyMetrics{}, err
	}
	delay, err := chronyValue(fields["Root delay"], " seconds")
	if err != nil {
		return chronyMetrics{}, err
	}
	dispersion, err := chronyValue(fields["Root dispersion"], " seconds")
	if err != nil {
		return chronyMetrics{}, err
	}
	skew, err := chronyValue(fields["Skew"], " ppm")
	if err != nil || delay < 0 || dispersion < 0 || skew < 0 {
		return chronyMetrics{}, errChronyEvidence
	}
	errorSeconds := math.Abs(offset) + dispersion + delay/2
	if !validClockNumber(errorSeconds) || errorSeconds > 60 {
		return chronyMetrics{}, errChronyEvidence
	}
	leap := fields["Leap status"]
	if leap == "" || fields["Reference ID"] == "" {
		return chronyMetrics{}, errChronyEvidence
	}
	referenceID := fields["Reference ID"]
	if left := strings.LastIndex(referenceID, "("); left >= 0 && strings.HasSuffix(referenceID, ")") {
		referenceID = referenceID[left+1 : len(referenceID)-1]
	}
	return chronyMetrics{errorBound: time.Duration(math.Ceil(errorSeconds * float64(time.Second))), skewPPM: skew, leap: leap, referenceTime: ref, referenceID: referenceID}, nil
}

func chronyValue(value, suffix string) (float64, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "+")
	parts := strings.Fields(value)
	if suffix == " seconds" {
		if len(parts) == 2 && parts[1] == "seconds" {
			value = parts[0]
		} else if len(parts) == 6 && parts[1] == "seconds" && (parts[2] == "fast" || parts[2] == "slow") &&
			parts[3] == "of" && parts[4] == "NTP" && parts[5] == "time" {
			value = parts[0]
		} else {
			return 0, errChronyEvidence
		}
	} else if suffix == " ppm" && len(parts) == 2 && parts[1] == "ppm" {
		value = parts[0]
	} else {
		return 0, errChronyEvidence
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || !validClockNumber(number) {
		return 0, errChronyEvidence
	}
	return number, nil
}

func selectedSources(output string, expected []string, referenceID string) bool {
	selected := make(map[string]byte, len(expected))
	referenceSelected := false
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || len(fields[0]) != 2 || (fields[0][1] != '*' && fields[0][1] != '+') {
			continue
		}
		if !validSelectedSource(fields, expected, selected) {
			return false
		}
		selected[fields[1]] = fields[0][1]
		if fields[1] == referenceID && fields[0][1] == '*' {
			referenceSelected = true
		}
	}
	if !referenceSelected {
		return false
	}
	for _, source := range expected {
		if selected[source] != '*' && selected[source] != '+' {
			return false
		}
	}
	return true
}

func validSelectedSource(fields []string, expected []string, selected map[string]byte) bool {
	return slices.Contains(expected, fields[1]) && selected[fields[1]] == 0 &&
		strings.ContainsRune("^=#", rune(fields[0][0])) && validReachability(fields[4]) && freshSourceSample(fields[5])
}

func validReachability(value string) bool {
	reach, err := strconv.ParseUint(value, 8, 8)
	return err == nil && reach > 0
}

func freshSourceSample(value string) bool {
	if value == "" {
		return false
	}
	multiplier := time.Second
	last := value[len(value)-1]
	switch last {
	case 'm':
		multiplier = time.Minute
	case 'h':
		multiplier = time.Hour
	case 'd':
		multiplier = 24 * time.Hour
	case 'y':
		multiplier = 365 * 24 * time.Hour
	default:
		if last < '0' || last > '9' {
			return false
		}
	}
	number, err := strconv.ParseUint(strings.TrimRight(value, "smhdy"), 10, 32)
	return err == nil && number <= uint64((60*time.Second)/multiplier)
}

func sameSources(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	set := make(map[string]bool, len(actual))
	for _, source := range actual {
		set[source] = true
	}
	for _, source := range expected {
		if !set[source] {
			return false
		}
	}
	return true
}

func freshReference(reference time.Time, utcSeconds int64) bool {
	age := time.Unix(utcSeconds, 0).Sub(reference)
	return age >= -2*time.Second && age <= 60*time.Second
}

type boundedOutput struct{ data []byte }

func (b *boundedOutput) Write(data []byte) (int, error) {
	if len(data) > chronyOutputLimit-len(b.data) {
		return 0, errChronyEvidence
	}
	b.data = append(b.data, data...)
	return len(data), nil
}

func runChronyCommand(ctx context.Context, executable string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- The operator pins executable; the adapter supplies fixed arguments.
	var output boundedOutput
	command.Stdout = &output
	command.Stderr = &boundedOutput{}
	if err := command.Run(); err != nil {
		return nil, err
	}
	return output.data, nil
}
