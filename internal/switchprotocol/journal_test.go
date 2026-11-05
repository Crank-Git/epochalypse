package switchprotocol

import (
	"bytes"
	"math"
	"testing"
)

func TestJournalIncludesDurableIntentBeforeTransmission(t *testing.T) {
	_, s := faultScenario(t)
	j, err := NewClientJournal(s.Initial)
	requireFaultStep(t, err)
	intent := SignedIntent{State: s.Next, Bytes: []byte("signed synthetic intent")}
	requireFaultStep(t, j.Issue(intent, true))
	if got := j.Bounds(); got.B0 != 110 || got.Bmax != 210 || got.Confirmed {
		t.Fatalf("durable unsent intent has wrong uncertainty: %+v", got)
	}
}

func TestRestartRetriesExactJournaledBytes(t *testing.T) {
	_, s := faultScenario(t)
	j, err := NewClientJournal(s.Initial)
	requireFaultStep(t, err)
	intent := SignedIntent{State: s.Next, Bytes: []byte("original signed bytes")}
	requireFaultStep(t, j.Issue(intent, true))
	intent.Bytes[0] = 'X'
	restarted := j.Restart()
	retry, err := restarted.Retry(s.Next.Sequence)
	requireFaultStep(t, err)
	if retry.State != s.Next || string(retry.Bytes) != "original signed bytes" {
		t.Fatal("restart changes signed intent")
	}
	retry.Bytes[0] = 'Y'
	again, err := restarted.Retry(s.Next.Sequence)
	requireFaultStep(t, err)
	if bytes.Equal(retry.Bytes, again.Bytes) {
		t.Fatal("retry exposes mutable journal bytes")
	}
	if _, err := restarted.Retry(999); err == nil {
		t.Fatal("retries an absent intent")
	}
}

func TestRejectsUndurableAndConflictingIntents(t *testing.T) {
	_, s := faultScenario(t)
	for _, change := range []func(*SignedIntent){
		func(i *SignedIntent) { i.State.SwitchID = "another" },
		func(i *SignedIntent) { i.State.Sequence = s.Initial.Sequence },
		func(i *SignedIntent) { i.State.Deadline = 90 },
		func(i *SignedIntent) { i.State.Deadline = math.MaxInt64 },
		func(i *SignedIntent) { i.Bytes = nil },
	} {
		j, err := NewClientJournal(s.Initial)
		requireFaultStep(t, err)
		i := SignedIntent{State: s.Next, Bytes: []byte("signed")}
		change(&i)
		if err := j.Issue(i, true); err == nil {
			t.Fatal("journals invalid intent")
		}
		if j.Bounds() != (Uncertainty{B0: 110, Bmax: 110, Confirmed: true}) {
			t.Fatal("rejection changes bounds")
		}
	}
	j, err := NewClientJournal(s.Initial)
	requireFaultStep(t, err)
	if err := j.Issue(SignedIntent{State: s.Next, Bytes: []byte("signed")}, false); err == nil {
		t.Fatal("transmits before persistence")
	}
	if _, err := j.Retry(s.Next.Sequence); err == nil {
		t.Fatal("crash retains volatile intent")
	}
	bad := s.Initial
	bad.Deadline = math.MaxInt64
	if _, err := NewClientJournal(bad); err == nil {
		t.Fatal("accepts overflowing initial boundary")
	}
}
