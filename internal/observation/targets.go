package observation

import (
	"errors"
	"net"
	"strconv"
	"strings"
)

// ValidateTargets bounds trusted resolver configuration before state or network access.
func ValidateTargets(targets []Target) error {
	if len(targets) == 0 || len(targets) > 3 {
		return errors.New("configure one to three resolver targets")
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if seen[target.ID] {
			return errors.New("resolver identifiers must be distinct")
		}
		seen[target.ID] = true
		if err := target.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (target Target) validate() error {
	for _, field := range []string{target.ID, target.Version, target.VersionSource, target.Clock.Source} {
		if field == "" || len(field) > 128 || strings.ContainsAny(field, "\r\n\x00") {
			return errors.New("provide bounded resolver identity and provenance")
		}
	}
	host, port, err := net.SplitHostPort(target.Address)
	if err != nil || host == "" || len(target.Address) > 253 || strings.ContainsAny(host, "/?#@ \t\r\n") {
		return errors.New("provide a configured resolver host and numeric port")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return errors.New("provide a configured resolver port from 1 through 65535")
	}
	return target.Clock.validate()
}

func (clock Clock) validate() error {
	if clock.Source == "unknown" && clock.OffsetNanoseconds != nil {
		return errors.New("an unknown clock offset cannot claim a value")
	}
	if (clock.LowerNanoseconds == nil) != (clock.UpperNanoseconds == nil) {
		return errors.New("provide both clock interval bounds")
	}
	if clock.LowerNanoseconds != nil && *clock.LowerNanoseconds > *clock.UpperNanoseconds {
		return errors.New("clock interval bounds are reversed")
	}
	if clock.Source != "unknown" && clock.OffsetNanoseconds == nil && clock.LowerNanoseconds == nil {
		return errors.New("provide clock provenance or an explicit unknown offset")
	}
	return nil
}
