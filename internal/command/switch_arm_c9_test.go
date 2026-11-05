package command

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func TestArmRecordMatchesRejectsInvalidDraftAndTerminalState(t *testing.T) {
	options, plan, key := armCommandFixture(t)
	frame, err := switchwire.DecodeFrame(plan.Prepare.Body)
	if err != nil {
		t.Fatal(err)
	}
	inputs := armInputs{plan: plan, key: key, id: frame.State.Switch}
	record, err := executeSwitch(options.stateDir, frame.State.Switch, switches.Input{}, window.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !armRecordMatches(record, inputs) {
		t.Fatal("matching draft rejects its signed arm plan")
	}

	for _, tc := range []struct {
		name   string
		change func(*switches.Record)
	}{
		{name: "invalid identifier", change: func(record *switches.Record) { record.ID = "invalid" }},
		{name: "terminal state", change: func(record *switches.Record) { record.State = "expired" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := record
			tc.change(&changed)
			if armRecordMatches(changed, inputs) {
				t.Fatal("invalid retained state accepts the signed arm plan")
			}
		})
	}
}

func TestRunSwitchArmRejectsMissingRetainedDraft(t *testing.T) {
	options, _, _ := armCommandFixture(t)
	options.stateDir = filepath.Join(t.TempDir(), "empty-state")
	var stdout, stderr bytes.Buffer
	if code := runSwitchArm(armCommandArguments(options)[2:], &stdout, &stderr); code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("missing draft: exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, path := range []string{options.bundlePath, options.bundlePath + ".intent", options.bundlePath + ".arm"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("missing draft writes arm evidence %q: %v", path, err)
		}
	}
}
