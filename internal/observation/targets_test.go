package observation

import (
	"strings"
	"testing"
)

func validTarget() Target {
	return Target{ID: "strict", Address: "127.0.0.1:1053", Version: "1.22.0", VersionSource: "configured", Clock: Clock{Source: "unknown"}}
}

func TestTargetsRejectUnboundedOrAmbiguousConfiguration(t *testing.T) {
	if ValidateTargets(nil) == nil || ValidateTargets(make([]Target, 4)) == nil {
		t.Fatal("an unbounded resolver count passes")
	}
	target := validTarget()
	if ValidateTargets([]Target{target, target}) == nil {
		t.Fatal("duplicate resolver identifiers pass")
	}
	zero, one := int64(0), int64(1)
	changes := []func(*Target){
		func(t *Target) { t.ID = "" },
		func(t *Target) { t.Version = strings.Repeat("x", 129) },
		func(t *Target) { t.VersionSource = "raw\nsource" },
		func(t *Target) { t.Address = "https://example.invalid/query" },
		func(t *Target) { t.Address = ":53" },
		func(t *Target) { t.Address = "user@example.invalid:53" },
		func(t *Target) { t.Address = "127.0.0.1:0" },
		func(t *Target) { t.Address = "127.0.0.1:65536" },
		func(t *Target) { t.Address = "127.0.0.1:dns" },
		func(t *Target) { t.Clock.OffsetNanoseconds = &zero },
		func(t *Target) { t.Clock.Source = "configured" },
		func(t *Target) { t.Clock.LowerNanoseconds = &zero },
		func(t *Target) { t.Clock.UpperNanoseconds = &zero },
		func(t *Target) { t.Clock.LowerNanoseconds = &one; t.Clock.UpperNanoseconds = &zero },
	}
	for _, change := range changes {
		target := validTarget()
		change(&target)
		if ValidateTargets([]Target{target}) == nil {
			t.Fatalf("an invalid target passes: %+v", target)
		}
	}
}

func TestTargetsPreserveExplicitClockProvenance(t *testing.T) {
	zero, one := int64(0), int64(1)
	for _, clock := range []Clock{{Source: "unknown"}, {Source: "configured", OffsetNanoseconds: &zero}, {Source: "measured interval", LowerNanoseconds: &zero, UpperNanoseconds: &one}} {
		target := validTarget()
		target.Clock = clock
		if err := ValidateTargets([]Target{target}); err != nil {
			t.Fatal(err)
		}
	}
}
