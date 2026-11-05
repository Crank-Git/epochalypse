package switchprotocol

import (
	"bytes"
	"errors"
)

// SignedIntent supplies authenticated synthetic bytes and their decoded state.
// The experiment assumes this association; it implements no signature or encoding.
type SignedIntent struct {
	State State
	Bytes []byte
}

type Uncertainty struct {
	B0        int64
	Bmax      int64
	Confirmed bool
}

// ClientJournal represents complete durable values, not a disk storage implementation.
type ClientJournal struct {
	confirmed State
	intents   []SignedIntent
	receipts  []Receipt
}

func NewClientJournal(initial State) (*ClientJournal, error) {
	if _, err := releaseBoundary(initial); err != nil {
		return nil, err
	}
	return &ClientJournal{confirmed: initial}, nil
}

// Issue records intent before any transmission can use it.
// persist=false models a crash before the journal commits the value.
func (j *ClientJournal) Issue(intent SignedIntent, persist bool) error {
	previous := j.confirmed
	if len(j.intents) != 0 {
		previous = j.intents[len(j.intents)-1].State
	}
	if !sameContent(previous, intent.State) || intent.State.Sequence <= previous.Sequence {
		return errors.New("intent identity or sequence conflicts")
	}
	boundary, err := releaseBoundary(intent.State)
	if err != nil {
		return err
	}
	old, _ := releaseBoundary(previous)
	if boundary < old || len(intent.Bytes) == 0 {
		return errors.New("intent boundary or signed bytes are invalid")
	}
	if !persist {
		return errors.New("intent is not durable")
	}
	j.intents = append(j.intents, cloneIntent(intent))
	return nil
}

// Retry returns the original durable bytes without a new deadline or sequence.
func (j *ClientJournal) Retry(sequence uint64) (SignedIntent, error) {
	for _, intent := range j.intents {
		if intent.State.Sequence == sequence {
			return cloneIntent(intent), nil
		}
	}
	return SignedIntent{}, errors.New("intent is not journaled")
}

// Restart reconstructs the client from complete synthetic durable values.
func (j *ClientJournal) Restart() *ClientJournal {
	copy := &ClientJournal{confirmed: j.confirmed, receipts: append([]Receipt(nil), j.receipts...)}
	for _, intent := range j.intents {
		copy.intents = append(copy.intents, cloneIntent(intent))
	}
	return copy
}

func (j *ClientJournal) receive(rules Rules, receipt Receipt) {
	j.receipts = append(j.receipts, receipt)
	if receipt.Accepted.Sequence > j.confirmed.Sequence && rules.RenewalConfirmed(receipt.Accepted, j.receipts) {
		j.confirmed = receipt.Accepted
	}
}

func (j *ClientJournal) matches(intent SignedIntent) bool {
	stored, err := j.Retry(intent.State.Sequence)
	return err == nil && stored.State == intent.State && bytes.Equal(stored.Bytes, intent.Bytes)
}

func (j *ClientJournal) Bounds() Uncertainty {
	b, _ := releaseBoundary(j.confirmed)
	result := Uncertainty{B0: b, Bmax: b, Confirmed: true}
	for _, intent := range j.intents {
		boundary, _ := releaseBoundary(intent.State)
		result.Bmax = max(result.Bmax, boundary)
		if intent.State.Sequence > j.confirmed.Sequence {
			result.Confirmed = false
		}
	}
	return result
}

func cloneIntent(intent SignedIntent) SignedIntent {
	intent.Bytes = bytes.Clone(intent.Bytes)
	return intent
}

func sameContent(a, b State) bool {
	return a.SwitchID == b.SwitchID && a.ProtocolVersion == b.ProtocolVersion && a.ContentDigest == b.ContentDigest
}
