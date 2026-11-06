package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func armedCheckinFixture(t *testing.T) (transitionFixture, *Transitions, switchwire.Frame) {
	t.Helper()
	f := newTransitionFixture(t)
	tr, err := NewTransitions(f.config(0))
	if err != nil {
		t.Fatal(err)
	}
	prepare := f.prepareRequest(t, f.material.Ciphertext)
	if _, err := tr.Prepare(prepare, f.material.Ciphertext, nil); err != nil {
		t.Fatal(err)
	}
	frame, err := switchwire.DecodeFrame(prepare.Body)
	if err != nil {
		t.Fatal(err)
	}
	frame.Operation, frame.Phase, frame.Entries = switchwire.OperationArm, switchwire.PhaseArmed, nil
	for i, a := range f.certificate.Agents {
		receipt, err := switchwire.SignFrame(switchwire.Frame{Operation: switchwire.OperationReceipt, Signer: a.ID, State: frame.State, Durable: 1, Phase: switchwire.PhasePrepared}, f.agentKeys[i])
		if err != nil {
			t.Fatal(err)
		}
		data, err := switchwire.MarshalEnvelope(receipt)
		if err != nil {
			t.Fatal(err)
		}
		frame.Entries = append(frame.Entries, switchwire.Entry{ID: a.ID, Key: a.SigningKey, Data: data})
	}
	arm, err := switchwire.SignFrame(frame, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Arm(arm); err != nil {
		t.Fatal(err)
	}
	frame.Operation, frame.Entries = switchwire.OperationCheckin, nil
	frame.State.Sequence, frame.State.Deadline = 2, 4700
	tr.clock = func() (Reading, error) { return Reading{lower: 1100}, nil }
	return f, tr, frame
}

func sendCheckin(t *testing.T, tr *Transitions, request switchwire.Envelope) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(transitionRequest{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	tr.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/agent/v1/switches/switch-1/checkins", bytes.NewReader(body)))
	return w
}

func TestCheckinPersistsSignedDeadlineAndRetriesAfterRestart(t *testing.T) {
	f, tr, frame := armedCheckinFixture(t)
	request, err := switchwire.SignFrame(frame, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	response := sendCheckin(t, tr, request)
	if response.Code != http.StatusOK {
		t.Fatalf("check-in status=%d body=%s", response.Code, response.Body)
	}
	state, err := f.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	exact, err := switchwire.MarshalEnvelope(request)
	if err != nil || !bytes.Equal(state.AcceptedRequest, exact) || len(state.Receipt) == 0 {
		t.Fatal("receipt precedes durable signed intent")
	}
	if err := f.journal.Close(); err != nil {
		t.Fatal(err)
	}
	f.journal, err = Open(f.authorityConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.journal.Close() })
	tr, err = NewTransitions(f.config(0))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateServiceState(tr, f.journal); err != nil {
		t.Fatalf("restart: %v", err)
	}
	tr.clock = func() (Reading, error) { return Reading{lower: 4500}, nil }
	retry := sendCheckin(t, tr, request)
	if retry.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), retry.Body.Bytes()) {
		t.Fatalf("retry changed receipt: %d %s", retry.Code, retry.Body)
	}
	retained, err := f.journal.Snapshot()
	if err != nil || retained.Generation != state.Generation {
		t.Fatal("arrival changed durable deadline")
	}
}

func TestCheckinRecoversCommittedIntentWithoutReceipt(t *testing.T) {
	f, tr, frame := armedCheckinFixture(t)
	request, err := switchwire.SignFrame(frame, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	fault := errors.New("synthetic receipt commit failure")
	seeks := 0
	f.journal.performIO = func(operation string, action func() error) error {
		if operation == "seek-journal" {
			seeks++
			if seeks == 2 {
				return fault
			}
		}
		return action()
	}
	receipt, err := tr.Checkin(request)
	if !errors.Is(err, fault) || len(receipt.Body) != 0 {
		t.Fatalf("failed receipt commit: %v", err)
	}
	state, err := f.journal.Snapshot()
	if err != nil || len(state.Receipt) != 0 {
		t.Fatal("receipt failure loses accepted intent")
	}
	if err := f.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := f.config(0)
	config.ReadClock = func() (Reading, error) { return Reading{lower: 1100}, nil }
	service, err := OpenService(ServiceConfig{Authority: f.authorityConfig, Transition: config, LabOnlySyntheticClock: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	receipt, err = service.transitions.Checkin(request)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := switchwire.DecodeFrame(receipt.Body)
	if err != nil || accepted.State != frame.State {
		t.Fatal("retry changes the committed request")
	}
}

func TestCheckinRejectsInvalidAuthorityAndTime(t *testing.T) {
	for _, name := range []string{"signature", "sequence", "deadline", "future", "boundary", "released", "unsafe", "phase", "switch"} {
		t.Run(name, func(t *testing.T) {
			f, tr, frame := armedCheckinFixture(t)
			before, err := f.journal.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "sequence":
				frame.State.Sequence = 1
			case "deadline":
				frame.State.Deadline = 4599
			case "future":
				frame.State.Deadline = 4705
			case "boundary":
				tr.clock = func() (Reading, error) { return Reading{lower: 4660}, nil }
			case "phase":
				frame.Phase = switchwire.PhasePrepared
			case "switch":
				frame.State.Switch = "another-switch"
			case "released", "unsafe":
				before.Generation++
				before.PossibleDisclosure = name == "released"
				before.UnsafeClock = name == "unsafe"
				if _, err := f.journal.Commit(before); err != nil {
					t.Fatal(err)
				}
			}
			request, err := switchwire.SignFrame(frame, f.ownerKey)
			if err != nil {
				t.Fatal(err)
			}
			if name == "signature" {
				request.Signature[0] ^= 1
			}
			response := sendCheckin(t, tr, request)
			if response.Code == http.StatusOK {
				t.Fatal("invalid check-in returns success")
			}
			after, err := f.journal.Snapshot()
			if err != nil || after.Generation != before.Generation {
				t.Fatal("invalid check-in changes authority")
			}
		})
	}
}
