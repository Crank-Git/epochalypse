package command

import (
	"os"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestExecuteArmRejectsInvalidRetainedIntent(t *testing.T) {
	options, plan, key := armCommandFixture(t)
	if err := os.WriteFile(options.bundlePath+".arm", []byte("invalid arm evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := switches.Record{State: switches.StateUncertain, Arm: &switches.ArmEvidence{}}
	frame, err := switchwire.DecodeFrame(plan.Prepare.Body)
	if err != nil {
		t.Fatal(err)
	}
	inputs := armInputs{plan: plan, key: key, id: frame.State.Switch}
	updated, _, armErr, operationErr := executeArm(nil, record, inputs, options.bundlePath)
	if operationErr == nil || armErr != nil || updated.State != record.State || updated.Arm != record.Arm {
		t.Fatalf("invalid retained intent was not rejected unchanged: state=%s armErr=%v operationErr=%v", updated.State, armErr, operationErr)
	}
}
