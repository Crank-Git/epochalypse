package contract

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"

	"filippo.io/age"
)

func fixture() (Message, ed25519.PrivateKey, map[string]ed25519.PublicKey) {
	k := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32))
	s := State{Switch: "synthetic", Version: 2, Sequence: 1, Deadline: 100, Grace: 10, Content: [32]byte{1}, Key: [32]byte{2}, Enrollment: [32]byte{3}}
	return Message{Operation: "checkin", Signer: "owner", State: s}, k, map[string]ed25519.PublicKey{"owner": k.Public().(ed25519.PublicKey), "agent-a": k.Public().(ed25519.PublicKey)}
}

func TestPreservesVersionTwoByteVector(t *testing.T) {
	m, _, _ := fixture()
	b, err := encode(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 231 || fmt.Sprintf("%x", sha256.Sum256(b)) != "a258f5ccfb10673bc7a30c48e6b7643116a1173de11c9358f5ec66cadadd1497" {
		t.Fatal("version two bytes differ")
	}
}

func signed(t *testing.T, m Message, k ed25519.PrivateKey) Envelope {
	t.Helper()
	b, err := encode(m)
	if err != nil {
		t.Fatal(err)
	}
	return Envelope{b, ed25519.Sign(k, b)}
}

func TestVerifiesEveryMessageDomain(t *testing.T) {
	for _, op := range []string{"prepare", "arm", "checkin", "status-request", "status-response", "release", "receipt", "cancel"} {
		t.Run(op, func(t *testing.T) {
			m, k, keys := fixture()
			m.Operation = op
			if op == "receipt" || op == "status-response" || op == "release" {
				m.Signer = "agent-a"
				m.Durable = 1
			}
			if op == "prepare" || op == "arm" {
				m.Entries = []Entry{{"agent-a", [32]byte{1}, []byte{1}}, {"agent-b", [32]byte{2}, []byte{2}}, {"agent-c", [32]byte{3}, []byte{3}}}
			}
			if op == "release" {
				m.Released = true
				m.Entries = []Entry{{"agent-a", [32]byte{1}, bytes.Repeat([]byte{5}, 33)}}
			}
			e := signed(t, m, k)
			got, err := verify(e, keys)
			if err != nil || got.State != m.State || got.Operation != op {
				t.Fatalf("domain or state differs: %v", err)
			}
			b, err := encode(got)
			if err != nil || !bytes.Equal(b, e.Body) {
				t.Fatal("canonical bytes differ")
			}
		})
	}
}

func TestRejectsAlteredSignedInput(t *testing.T) {
	m, k, keys := fixture()
	e := signed(t, m, k)
	for i := range e.Body {
		b := bytes.Clone(e.Body)
		b[i] ^= 1
		if _, err := verify(Envelope{b, e.Signature}, keys); err == nil {
			t.Fatalf("accepts altered byte %d", i)
		}
	}
	for i := 0; i < len(e.Body); i++ {
		if _, err := verify(Envelope{e.Body[:i], e.Signature}, keys); err == nil {
			t.Fatalf("accepts truncation %d", i)
		}
	}
	for _, mutate := range []func(*Message){
		func(m *Message) { m.State.Version = 3 }, func(m *Message) { m.Operation = "unknown" }, func(m *Message) { m.Signer = "stranger" },
		func(m *Message) { m.Signer = "agent-a" }, func(m *Message) { m.State.Switch = string(bytes.Repeat([]byte{'x'}, 129)) },
		func(m *Message) { m.State.Grace = ^uint64(0) }, func(m *Message) { m.State.Deadline = -1 }, func(m *Message) { m.State.Sequence = 0 },
	} {
		n := m
		mutate(&n)
		b, err := encode(n)
		if err == nil {
			if _, err = verify(Envelope{b, ed25519.Sign(k, b)}, keys); err == nil {
				t.Fatal("accepts invalid message")
			}
		}
	}
	wrong := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, 32))
	if _, err := verify(Envelope{e.Body, ed25519.Sign(wrong, e.Body)}, keys); err == nil {
		t.Fatal("accepts wrong key")
	}
	if _, err := verify(Envelope{append(bytes.Clone(e.Body), 0), e.Signature}, keys); err == nil {
		t.Fatal("accepts trailing bytes")
	}
	if _, err := verify(Envelope{make([]byte, 16385), e.Signature}, keys); err == nil {
		t.Fatal("accepts oversized body")
	}
	keys["owner"] = nil
	if _, err := verify(e, keys); err == nil {
		t.Fatal("accepts malformed key")
	}
}

func TestRejectsNoncanonicalAndUnboundedEntries(t *testing.T) {
	m, _, _ := fixture()
	m.Operation = "prepare"
	good := []Entry{{"agent-a", [32]byte{1}, []byte{1}}, {"agent-b", [32]byte{2}, []byte{2}}, {"agent-c", [32]byte{3}, []byte{3}}}
	for _, entries := range [][]Entry{nil, good[:2], {good[1], good[0], good[2]}, {good[0], good[0], good[2]}, {{"agent-a", [32]byte{1}, make([]byte, 4097)}, good[1], good[2]}} {
		m.Entries = entries
		if _, err := encode(m); err == nil {
			t.Fatal("accepts invalid entries")
		}
	}
	m.Entries = good
	b, err := encode(m)
	if err != nil {
		t.Fatal(err)
	}
	// The length limit applies before the decoder allocates an entry.
	binary.BigEndian.PutUint32(b[len(b)-5:len(b)-1], 0xffffffff)
	k := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32))
	keys := map[string]ed25519.PublicKey{"owner": k.Public().(ed25519.PublicKey)}
	if _, err := verify(Envelope{b, ed25519.Sign(k, b)}, keys); err == nil {
		t.Fatal("accepts an unbounded field")
	}
}

func TestBindsJournalBytesToStateAndEnrollment(t *testing.T) {
	m, k, keys := fixture()
	e := signed(t, m, k)
	if err := journal(e, m.State, keys); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*State){func(s *State) { s.Switch = "other" }, func(s *State) { s.Sequence++ }, func(s *State) { s.Deadline++ }, func(s *State) { s.Grace++ }, func(s *State) { s.Content[0]++ }, func(s *State) { s.Key[0]++ }, func(s *State) { s.Enrollment[0]++ }, func(s *State) { s.Version++ }} {
		s := m.State
		change(&s)
		if err := journal(e, s, keys); err == nil {
			t.Fatal("accepts mismatched decoded state")
		}
	}
	copy := Envelope{bytes.Clone(e.Body), bytes.Clone(e.Signature)}
	if err := journal(copy, m.State, keys); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copy.Body, e.Body) {
		t.Fatal("retry changes bytes")
	}
}

func TestRejectsReplayAndSequenceConflict(t *testing.T) {
	m, _, _ := fixture()
	old := m.State
	if err := accept(old, old); err == nil {
		t.Fatal("accepts replay as new authority")
	}
	conflict := old
	conflict.Deadline++
	if err := accept(old, conflict); err == nil {
		t.Fatal("accepts same-sequence conflict")
	}
	next := old
	next.Sequence++
	next.Deadline++
	if err := accept(old, next); err != nil {
		t.Fatal(err)
	}
	next.Deadline = 99
	if err := accept(old, next); err == nil {
		t.Fatal("shortens boundary")
	}
	next = old
	next.Sequence++
	next.Key[0]++
	if err := accept(old, next); err == nil {
		t.Fatal("changes key identity")
	}
}

func TestQuarantinesBeforeReceiptsOrShares(t *testing.T) {
	m, _, _ := fixture()
	s := Snapshot{State: m.State, Phase: 2, Generation: 2}
	a := Authority{s, true, true}
	if err := restore(s, a); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Authority){func(a *Authority) { a.Present = false }, func(a *Authority) { a.Complete = false }, func(a *Authority) { a.Head.Generation++ }, func(a *Authority) { a.Head.State.Deadline++ }, func(a *Authority) { a.Head.Released = true }, func(a *Authority) { a.Head.Unsafe = true }} {
		n := a
		change(&n)
		if err := restore(s, n); err == nil {
			t.Fatal("accepts missing, stale, or inconsistent evidence")
		}
	}
}

func TestLifecyclePersistsBeforeOutput(t *testing.T) {
	m, _, _ := fixture()
	s := Snapshot{State: m.State}
	for _, action := range []string{"prepare", "arm", "release"} {
		before := s
		if got, out, err := transition(s, action, false, true); err == nil || out || got != before {
			t.Fatal("outputs before durable commit")
		}
		var out bool
		var err error
		s, out, err = transition(s, action, true, true)
		if err != nil || !out {
			t.Fatalf("transition %s: %v", action, err)
		}
		if action == "release" && !s.Released {
			t.Fatal("disclosure lacks durable latch")
		}
	}
	for _, action := range []string{"renew", "cancel", "arm", "replace"} {
		if _, _, err := transition(s, action, true, true); err == nil {
			t.Fatal("recalls or renews a disclosed key")
		}
	}
	if _, out, err := transition(s, "restart", true, false); err == nil || out {
		t.Fatal("restart skips fresh assessment")
	}
	got, out, err := transition(s, "restart", true, true)
	if err != nil || out || !got.Released {
		t.Fatal("restart loses release latch")
	}
}

func TestCancelAndReplacementPreserveOldAuthority(t *testing.T) {
	m, _, _ := fixture()
	s := Snapshot{State: m.State, Phase: 2}
	cancelled, out, err := transition(s, "cancel", true, true)
	if err != nil || !out || cancelled.Phase != 3 || cancelled.State != s.State {
		t.Fatal("cancel changes deadline or loses receipt")
	}
	if _, _, err := transition(cancelled, "renew", true, true); err == nil {
		t.Fatal("renews cancelled key")
	}
	if _, _, err := transition(s, "replace", true, true); err == nil {
		t.Fatal("replaces threshold slot for same key")
	}
	if _, _, err := transition(s, "arm", true, true); err == nil {
		t.Fatal("rearms existing key")
	}
	if released, out, err := transition(cancelled, "release", true, true); err != nil || !out || !released.Released {
		t.Fatal("cancel prevents independent release")
	}
}

func TestRejectsExhaustedGeneration(t *testing.T) {
	s := Snapshot{Generation: ^uint64(0)}
	if got, out, err := transition(s, "prepare", true, true); err == nil || out || got != s {
		t.Fatal("generation wraps before output")
	}
}

func TestRequiresThreeMatchingAuthenticatedReceipts(t *testing.T) {
	m, k, keys := fixture()
	agents := [3]string{"agent-a", "agent-b", "agent-c"}
	var receipts []Envelope
	for i, id := range agents {
		seed := bytes.Repeat([]byte{byte(i + 10)}, 32)
		key := ed25519.NewKeyFromSeed(seed)
		keys[id] = key.Public().(ed25519.PublicKey)
		m.Signer = id
		m.Operation = "receipt"
		m.Phase = 2
		m.Durable = 1
		receipts = append(receipts, signed(t, m, key))
	}
	for mask := 0; mask < 8; mask++ {
		var subset []Envelope
		for i, e := range receipts {
			if mask&(1<<i) != 0 {
				subset = append(subset, e)
			}
		}
		if confirm(m.State, subset, keys, agents) != (mask == 7) {
			t.Fatalf("confirmation differs for mask %d", mask)
		}
	}
	if confirm(m.State, []Envelope{receipts[0], receipts[0], receipts[1]}, keys, agents) {
		t.Fatal("duplicate receipts confirm")
	}
	wrong := m.State
	wrong.Enrollment[0]++
	if confirm(wrong, receipts, keys, agents) {
		t.Fatal("old enrollment confirms")
	}
	if confirm(m.State, receipts, keys, [3]string{"agent-a", "agent-a", "agent-c"}) {
		t.Fatal("repeated enrollment confirms")
	}
	prepared := m
	prepared.Phase = 1
	prepared.Signer = "agent-a"
	preparedKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{10}, 32))
	if confirm(m.State, []Envelope{signed(t, prepared, preparedKey), receipts[1], receipts[2]}, keys, agents) {
		t.Fatal("prepared receipt confirms armed state")
	}
	m.Operation = "checkin"
	m.Signer = "owner"
	m.Durable = 0
	if confirm(m.State, []Envelope{receipts[0], receipts[1], signed(t, m, k)}, keys, agents) {
		t.Fatal("owner message substitutes for receipt")
	}
}

func enrollmentAgents(t *testing.T) [3]enrolledAgent {
	t.Helper()
	var agents [3]enrolledAgent
	for i, id := range [3]string{"agent-a", "agent-b", "agent-c"} {
		key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{byte(i + 10)}, 32))
		recipient, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		agents[i] = enrolledAgent{ID: id, SigningKey: [32]byte(key.Public().(ed25519.PublicKey)), Recipient: recipient.Recipient().String()}
	}
	return agents
}

func TestRejectsRepeatedEnrollmentKeysAndRecipients(t *testing.T) {
	agents := enrollmentAgents(t)
	for _, duplicate := range []string{"signature-key", "recipient"} {
		t.Run(duplicate, func(t *testing.T) {
			n := agents
			if duplicate == "signature-key" {
				n[1].SigningKey = n[0].SigningKey
			} else {
				n[1].Recipient = n[0].Recipient
			}
			if err := validateEnrollment(n); err == nil {
				t.Fatal("accepts repeated enrollment key material")
			}
		})
	}
}

func TestRejectsOneSignatureKeyUnderThreeAgentIdentities(t *testing.T) {
	m, key, _ := fixture()
	keys := map[string]ed25519.PublicKey{}
	ids := [3]string{"agent-a", "agent-b", "agent-c"}
	var receipts []Envelope
	for _, id := range ids {
		keys[id] = key.Public().(ed25519.PublicKey)
		m.Operation = "receipt"
		m.Signer = id
		m.Phase = 2
		m.Durable = m.State.Sequence
		receipts = append(receipts, signed(t, m, key))
	}
	if confirm(m.State, receipts, keys, ids) {
		t.Fatal("one signature key confirms three agents")
	}
}

func TestRejectsInvalidWireBranches(t *testing.T) {
	m, k, keys := fixture()
	for _, change := range []func(*Message){
		func(m *Message) { m.Phase = 4 }, func(m *Message) { m.Operation = "receipt"; m.Signer = "agent-a" },
		func(m *Message) { m.Operation = "release"; m.Signer = "agent-a" }, func(m *Message) { m.Entries = []Entry{{"agent-a", [32]byte{}, []byte{1}}} },
		func(m *Message) { m.Operation = "receipt"; m.Durable = 1 }, func(m *Message) { m.State.Switch = "invalid!" },
		func(m *Message) {
			m.Operation = "prepare"
			m.Entries = []Entry{{"a", [32]byte{}, nil}, {"b", [32]byte{}, nil}, {"c", [32]byte{}, nil}}
		},
	} {
		n := m
		change(&n)
		if _, err := encode(n); err == nil {
			t.Fatal("accepts invalid operation fields")
		}
	}
	e := signed(t, m, k)
	for _, b := range [][]byte{[]byte(domain + "unknown\x00"), []byte(domain + "checkin"), []byte(domain + "operationtoolonggggg\x00")} {
		if _, err := verify(Envelope{b, e.Signature}, keys); err == nil {
			t.Fatal("accepts invalid operation domain")
		}
	}
	b := bytes.Clone(e.Body)
	b[len(b)-5] = 2
	if _, err := verify(Envelope{b, ed25519.Sign(k, b)}, keys); err == nil {
		t.Fatal("accepts invalid flag")
	}
	if _, err := verify(Envelope{e.Body, nil}, keys); err == nil {
		t.Fatal("accepts empty signature")
	}
	if err := journal(Envelope{[]byte("corrupt"), nil}, m.State, keys); err == nil {
		t.Fatal("accepts corrupt journal")
	}
	for _, phase := range []byte{0, 1, 3} {
		s := Snapshot{State: m.State, Phase: phase}
		for _, op := range []string{"cancel", "release"} {
			if phase == 3 && op == "release" {
				continue
			}
			if _, _, err := transition(s, op, true, true); err == nil {
				t.Fatal("accepts invalid lifecycle")
			}
		}
	}
	for _, s := range []Snapshot{{Phase: 2}, {Phase: 1, Released: true}, {Unsafe: true}} {
		if _, _, err := transition(s, "prepare", true, true); err == nil {
			t.Fatal("prepares existing or unsafe key")
		}
	}
}
