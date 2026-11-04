package switchprotocol

import "errors"

// ClockReading separates UTC protocol time from monotonic elapsed seconds.
// Safe asserts an external clock assessment that this model cannot measure.
type ClockReading struct {
	UTC     int64
	Elapsed uint64
	Safe    bool
}

// ModelAgent applies candidate local rules to authenticated synthetic check-ins.
// It supplies no persistence, receipt, signature, or production release operation.
type ModelAgent struct {
	accepted  State
	utc       int64
	elapsed   uint64
	tolerance uint64
	unsafe    bool
	released  bool
}

// NewModelAgent returns a local model with a declared clock tolerance in seconds.
// It rejects an unsafe initial clock or an unrepresentable release boundary.
func NewModelAgent(state State, reading ClockReading, tolerance uint64) (*ModelAgent, error) {
	if _, err := releaseBoundary(state); err != nil {
		return nil, err
	}
	if !reading.Safe {
		return nil, errors.New("initial clock is unsafe")
	}
	return &ModelAgent{accepted: state, utc: reading.UTC, elapsed: reading.Elapsed, tolerance: tolerance}, nil
}

// Accepted returns the last accepted state without a mutable reference.
func (a *ModelAgent) Accepted() State { return a.accepted }

// CheckIn accepts an increasing sequence before the current local release boundary.
// The caller authenticates the owner and serializes calls before this model transition.
// A rejection preserves the accepted state but can latch clock or release status.
func (a *ModelAgent) CheckIn(next State, reading ClockReading) error {
	eligible, err := a.ReleaseEligible(reading)
	if err != nil {
		return err
	}
	if eligible {
		return errors.New("local release boundary reached")
	}
	if next.Sequence <= a.accepted.Sequence {
		return errors.New("sequence does not increase")
	}
	if next.SwitchID != a.accepted.SwitchID || next.ProtocolVersion != a.accepted.ProtocolVersion || next.ContentDigest != a.accepted.ContentDigest {
		return errors.New("check-in conflicts with switch identity")
	}
	if _, err := releaseBoundary(next); err != nil {
		return err
	}
	a.accepted = next
	return nil
}

// ReleaseEligible reports whether the model reaches its accepted release boundary.
// Unsafe clocks block both check-in and new release evidence until external review.
// A later check-in cannot clear the release latch.
func (a *ModelAgent) ReleaseEligible(reading ClockReading) (bool, error) {
	if err := a.advance(reading); err != nil {
		return false, err
	}
	boundary, _ := releaseBoundary(a.accepted)
	a.released = a.released || a.utc >= boundary
	return a.released, nil
}

func (a *ModelAgent) advance(reading ClockReading) error {
	if a.unsafe || !reading.Safe || reading.Elapsed < a.elapsed {
		a.unsafe = true
		return errors.New("clock is unsafe")
	}
	projected, err := addSeconds(a.utc, reading.Elapsed-a.elapsed)
	if err != nil || distance(projected, reading.UTC) > a.tolerance {
		a.unsafe = true
		return errors.New("clock exceeds modeled tolerance")
	}
	// Elapsed time prevents a backward UTC step from reopening an earlier boundary.
	a.utc = max(projected, reading.UTC)
	a.elapsed = reading.Elapsed
	return nil
}

func releaseBoundary(state State) (int64, error) {
	return addSeconds(state.Deadline, state.Grace)
}

func addSeconds(utc int64, seconds uint64) (int64, error) {
	// Ordered unsigned values cover the full signed UTC range without signed overflow.
	ordered := uint64(utc) ^ (uint64(1) << 63) // #nosec G115 -- The conversion preserves the signed UTC bit pattern.
	if seconds > ^uint64(0)-ordered {
		return 0, errors.New("UTC boundary overflows")
	}
	return int64((ordered + seconds) ^ (uint64(1) << 63)), nil // #nosec G115 -- The conversion restores the signed UTC bit pattern.
}

func distance(a, b int64) uint64 {
	x, y := uint64(a)^(uint64(1)<<63), uint64(b)^(uint64(1)<<63) // #nosec G115 -- The conversions preserve signed UTC bit patterns.
	if x > y {
		return x - y
	}
	return y - x
}
