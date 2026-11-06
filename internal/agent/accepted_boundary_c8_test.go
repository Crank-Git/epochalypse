package agent

import (
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestOpenServiceRejectsArmWithoutMatchingPreparedReceipts(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	prepareRequest := fixture.prepareRequest(t, fixture.material.Ciphertext)
	if _, err := transitions.Prepare(prepareRequest, fixture.material.Ciphertext, nil); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := switchwire.VerifyOwnerRequest(prepareRequest, fixture.enrollment)
	if err != nil {
		t.Fatal(err)
	}
	arm, err := fixture.armRequestWithMismatchedReceipts(t, prepared.State)
	if err != nil {
		t.Fatal(err)
	}
	state.Phase = switchwire.PhaseArmed
	state.AcceptedRequest, err = switchwire.MarshalEnvelope(arm)
	if err != nil {
		t.Fatal(err)
	}
	state.Receipt = nil
	if _, err := transitions.commit(state); err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	clockCalls := 0
	config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
	config.Transition.ReadClock = func() (Reading, error) {
		clockCalls++
		return Reading{lower: 1000}, nil
	}
	if _, err := OpenService(config); err == nil {
		t.Fatal("service accepted an arm without three matching prepared receipts")
	}
	if clockCalls != 0 {
		t.Fatal("service assessed the clock before it rejected the arm")
	}
}

func TestOpenServiceReopensArmedStateWithoutReceipt(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	prepareAndArmFixture(t, fixture, transitions)
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	state.Receipt = nil
	if _, err := transitions.commit(state); err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
	service, err := OpenService(config)
	if err != nil {
		t.Fatalf("service rejected durable arm without receipt: %v", err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewTransitionsRejectsInvalidLocalConfiguration(t *testing.T) {
	for name, change := range map[string]func(*TransitionConfig){
		"missing journal":      func(config *TransitionConfig) { config.Journal = nil },
		"invalid owner key":    func(config *TransitionConfig) { config.OwnerKey = nil },
		"missing agent id":     func(config *TransitionConfig) { config.AgentID = "" },
		"invalid age identity": func(config *TransitionConfig) { config.AgeIdentity = "invalid" },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			config := fixture.config(0)
			change(&config)
			transitions, err := NewTransitions(config)
			if err == nil || transitions != nil {
				t.Fatalf("constructor returned transitions=%v, error=%v", transitions, err)
			}
		})
	}
}

func TestOpenServiceRejectsEnrollmentBeforeClockEvidence(t *testing.T) {
	fixture := newTransitionFixture(t)
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	clockCalls := 0
	config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
	config.Transition.EnrollmentSignature = append([]byte(nil), config.Transition.EnrollmentSignature...)
	config.Transition.EnrollmentSignature[0] ^= 1
	config.Transition.ReadClock = func() (Reading, error) {
		clockCalls++
		return Reading{lower: 1000}, nil
	}
	if _, err := OpenService(config); err == nil {
		t.Fatal("service accepted an enrollment with an invalid signature")
	}
	if clockCalls != 0 {
		t.Fatal("service assessed clock evidence before it rejected enrollment")
	}
}

func (f transitionFixture) armRequestWithMismatchedReceipts(t *testing.T, state switchwire.State) (switchwire.Envelope, error) {
	t.Helper()
	entries := make([]switchwire.Entry, 0, len(f.certificate.Agents))
	for index, agent := range f.certificate.Agents {
		receiptState := state
		receiptState.Sequence++
		receipt, err := switchwire.SignFrame(switchwire.Frame{
			Operation: switchwire.OperationReceipt, Signer: agent.ID, State: receiptState,
			Durable: receiptState.Sequence, Phase: switchwire.PhasePrepared,
		}, f.agentKeys[index])
		if err != nil {
			return switchwire.Envelope{}, err
		}
		encoded, err := switchwire.MarshalEnvelope(receipt)
		if err != nil {
			return switchwire.Envelope{}, err
		}
		entries = append(entries, switchwire.Entry{ID: agent.ID, Key: agent.SigningKey, Data: encoded})
	}
	arm, err := switchwire.SignFrame(switchwire.Frame{
		Operation: switchwire.OperationArm, Signer: "owner", State: state, Phase: switchwire.PhaseArmed, Entries: entries,
	}, f.ownerKey)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	return arm, nil
}
