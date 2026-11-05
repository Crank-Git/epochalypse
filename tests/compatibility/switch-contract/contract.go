// Package contract supplies isolated synthetic protocol evidence for review.
package contract

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"filippo.io/age"
)

type State struct {
	Switch                   string
	Version                  uint32
	Sequence                 uint64
	Deadline                 int64
	Grace                    uint64
	Content, Key, Enrollment [32]byte
}

type Entry struct {
	ID   string
	Key  [32]byte
	Data []byte
}
type Message struct {
	Operation, Signer string
	State             State
	Nonce             [32]byte
	Durable           uint64
	Phase             byte
	Released          bool
	Entries           []Entry
}
type Envelope struct{ Body, Signature []byte }

type enrolledAgent struct {
	ID         string
	SigningKey [32]byte
	Recipient  string
}

type admittedEnrollment struct {
	digest [32]byte
	agents [3]enrolledAgent
	valid  bool
}

func validateEnrollment(agents [3]enrolledAgent) error {
	keys := map[[32]byte]bool{}
	recipients := map[string]bool{}
	previousID := ""
	for _, agent := range agents {
		if !validID(agent.ID) || agent.ID == "owner" || previousID >= agent.ID {
			return errors.New("invalid or unordered enrolled identity")
		}
		if agent.SigningKey == [32]byte{} || keys[agent.SigningKey] {
			return errors.New("empty or repeated enrolled signature key")
		}
		if err := validateRecipient(agent.Recipient); err != nil {
			return err
		}
		if recipients[agent.Recipient] {
			return errors.New("repeated enrolled age recipient")
		}
		keys[agent.SigningKey] = true
		recipients[agent.Recipient] = true
		previousID = agent.ID
	}
	return nil
}

func validateRecipient(value string) error {
	if len(value) > 128 {
		return errors.New("enrolled age recipient exceeds limit")
	}
	r, err := age.ParseX25519Recipient(value)
	if err != nil {
		return fmt.Errorf("parse enrolled age recipient: %w", err)
	}
	if r.String() != value {
		return errors.New("enrolled age recipient is not canonical")
	}
	return nil
}

func admitEnrollment(digest [32]byte, agents [3]enrolledAgent) (admittedEnrollment, error) {
	if err := validateEnrollment(agents); err != nil {
		return admittedEnrollment{}, err
	}
	return admittedEnrollment{digest: digest, agents: agents, valid: true}, nil
}

const domain = "epochalypse/switch/research-v2/"
const maxBody = 16384

func validID(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for _, c := range []byte(s) {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
		if !valid {
			return false
		}
	}
	return true
}

func validate(m Message) error {
	if err := validateState(m); err != nil {
		return err
	}
	if m.Phase > 3 || len(m.Entries) > 3 {
		return errors.New("invalid phase or entry count")
	}
	if err := validateOperation(m); err != nil {
		return err
	}
	for i, e := range m.Entries {
		if !validID(e.ID) || len(e.Data) == 0 || len(e.Data) > 4096 || i > 0 && m.Entries[i-1].ID >= e.ID {
			return errors.New("invalid or unordered entry")
		}
	}
	return nil
}

func validateState(m Message) error {
	if !validID(m.Signer) || !validID(m.State.Switch) || m.State.Version != 2 || m.State.Sequence == 0 || m.State.Deadline < 0 || m.State.Grace > math.MaxInt64-uint64(m.State.Deadline) {
		return errors.New("invalid identity, version, sequence, or boundary")
	}
	return nil
}

func validateOperation(m Message) error {
	agent := false
	switch m.Operation {
	case "prepare", "arm":
		if len(m.Entries) != 3 {
			return errors.New("operation requires three entries")
		}
	case "checkin", "cancel", "status-request":
		if len(m.Entries) != 0 {
			return errors.New("operation requires no entries")
		}
	case "receipt", "status-response":
		agent = true
		if len(m.Entries) != 0 || m.Durable != m.State.Sequence {
			return errors.New("receipt sequence differs")
		}
	case "release":
		agent = true
		if !m.Released || m.Durable != m.State.Sequence || len(m.Entries) != 1 || m.Entries[0].ID != m.Signer || len(m.Entries[0].Data) != 33 {
			return errors.New("release evidence differs")
		}
	default:
		return errors.New("unknown operation")
	}
	if agent == (m.Signer == "owner") {
		return errors.New("signer role differs")
	}
	return nil
}

func putField(b *bytes.Buffer, v []byte) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(v))) // #nosec G115 -- validate bounds every field to at most 4096 bytes.
	b.Write(n[:])
	b.Write(v)
}

func encode(m Message) ([]byte, error) {
	if err := validate(m); err != nil {
		return nil, err
	}
	b := new(bytes.Buffer)
	b.WriteString(domain)
	b.WriteString(m.Operation)
	b.WriteByte(0)
	putField(b, []byte(m.State.Switch))
	// A buffer write cannot fail; fixed widths prevent alternate integer encodings.
	for _, v := range []any{m.State.Version, m.State.Sequence, m.State.Deadline, m.State.Grace} {
		_ = binary.Write(b, binary.BigEndian, v)
	}
	b.Write(m.State.Content[:])
	b.Write(m.State.Key[:])
	b.Write(m.State.Enrollment[:])
	putField(b, []byte(m.Signer))
	b.Write(m.Nonce[:])
	_ = binary.Write(b, binary.BigEndian, m.Durable)
	b.WriteByte(m.Phase)
	flag := byte(0)
	if m.Released {
		flag = 1
	}
	b.WriteByte(flag)
	_ = binary.Write(b, binary.BigEndian, uint32(len(m.Entries))) // #nosec G115 -- validate limits the entry count to three.
	for _, e := range m.Entries {
		putField(b, []byte(e.ID))
		b.Write(e.Key[:])
		putField(b, e.Data)
	}
	if b.Len() > maxBody {
		return nil, errors.New("body exceeds limit")
	}
	return b.Bytes(), nil
}

type decoder struct {
	r   *bytes.Reader
	err error
}

func (d *decoder) fixed(dst any) {
	if d.err == nil {
		d.err = binary.Read(d.r, binary.BigEndian, dst)
	}
}
func (d *decoder) field(limit uint32) []byte {
	var n uint32
	d.fixed(&n)
	if d.err != nil {
		return nil
	}
	if n > limit || int64(n) > int64(d.r.Len()) {
		d.err = errors.New("field exceeds limit")
		return nil
	}
	b := make([]byte, n)
	_, d.err = io.ReadFull(d.r, b)
	return b
}

func decode(body []byte) (Message, error) {
	var m Message
	if len(body) > maxBody || !bytes.HasPrefix(body, []byte(domain)) {
		return m, errors.New("invalid domain or body size")
	}
	rest := body[len(domain):]
	end := bytes.IndexByte(rest, 0)
	if end < 0 || end > 16 {
		return m, errors.New("invalid operation")
	}
	m.Operation = string(rest[:end])
	d := decoder{r: bytes.NewReader(rest[end+1:])}
	m.State.Switch = string(d.field(128))
	d.fixed(&m.State.Version)
	d.fixed(&m.State.Sequence)
	d.fixed(&m.State.Deadline)
	d.fixed(&m.State.Grace)
	d.fixed(&m.State.Content)
	d.fixed(&m.State.Key)
	d.fixed(&m.State.Enrollment)
	m.Signer = string(d.field(128))
	d.fixed(&m.Nonce)
	d.fixed(&m.Durable)
	d.fixed(&m.Phase)
	var flag byte
	d.fixed(&flag)
	m.Released = flag == 1
	var count uint32
	d.fixed(&count)
	if count > 3 || flag > 1 {
		return m, errors.New("invalid count or flag")
	}
	for range count {
		e := Entry{ID: string(d.field(128))}
		d.fixed(&e.Key)
		e.Data = d.field(4096)
		m.Entries = append(m.Entries, e)
	}
	if d.err != nil {
		return m, fmt.Errorf("decode body: %w", d.err)
	}
	if d.r.Len() != 0 {
		return m, errors.New("trailing bytes")
	}
	return m, validate(m)
}

func verify(e Envelope, keys map[string]ed25519.PublicKey) (Message, error) {
	m, err := decode(e.Body)
	if err != nil {
		return Message{}, err
	}
	k := keys[m.Signer]
	if len(k) != ed25519.PublicKeySize || len(e.Signature) != ed25519.SignatureSize || !ed25519.Verify(k, e.Body, e.Signature) {
		return Message{}, errors.New("signature or enrolled identity differs")
	}
	return m, nil
}

func journal(e Envelope, claimed State, keys map[string]ed25519.PublicKey) error {
	m, err := verify(e, keys)
	if err != nil {
		return err
	}
	if m.Operation != "checkin" || m.State != claimed {
		return errors.New("journal bytes differ from requested state")
	}
	return nil
}

func accept(old State, next State) error {
	a, b := old, next
	a.Sequence = 0
	b.Sequence = 0
	a.Deadline = 0
	b.Deadline = 0
	if a != b || next.Sequence <= old.Sequence || next.Deadline < old.Deadline {
		return errors.New("identity, sequence, or boundary conflicts")
	}
	return nil
}

type Snapshot struct {
	State            State
	Phase            byte
	Released, Unsafe bool
	Generation       uint64
}
type Authority struct {
	Head     Snapshot
	Complete bool
	Present  bool
}

func restore(s Snapshot, a Authority) error {
	if !a.Present || !a.Complete || a.Head != s {
		return errors.New("quarantine: authoritative state is absent or differs")
	}
	return nil
}

func transition(s Snapshot, action string, durable bool, fresh bool) (Snapshot, bool, error) {
	if !fresh || s.Unsafe {
		return s, false, errors.New("clock or freshness assessment blocks output")
	}
	if action == "restart" {
		return s, false, nil
	}
	if !durable {
		return s, false, errors.New("durable commit is absent")
	}
	if s.Generation == math.MaxUint64 {
		return s, false, errors.New("durable generation is exhausted")
	}
	n, err := successor(s, action)
	if err != nil {
		return s, false, err
	}
	n.Generation++
	return n, true, nil
}

func successor(s Snapshot, action string) (Snapshot, error) {
	n := s
	switch action {
	case "prepare":
		if s.Phase != 0 || s.Released {
			return s, errors.New("prepare requires new content")
		}
		n.Phase = 1
	case "arm":
		if s.Phase != 1 || s.Released {
			return s, errors.New("arm requires prepared content")
		}
		n.Phase = 2
	case "release":
		if s.Phase != 2 && s.Phase != 3 {
			return s, errors.New("release requires armed content")
		}
		n.Released = true
	case "cancel":
		if s.Phase != 2 || s.Released {
			return s, errors.New("cancel cannot recall a share")
		}
		n.Phase = 3
	default:
		return s, errors.New("transition requires a different content key or valid operation")
	}
	return n, nil
}

func confirm(state State, receipts []Envelope, enrollment admittedEnrollment) bool {
	if !enrollment.valid || state.Enrollment != enrollment.digest {
		return false
	}
	keys := map[string]ed25519.PublicKey{}
	for _, agent := range enrollment.agents {
		keys[agent.ID] = agent.SigningKey[:]
	}
	seen := map[string]bool{}
	for _, e := range receipts {
		m, err := verify(e, keys)
		if err != nil || m.Operation != "receipt" || m.State != state || m.Phase != 2 {
			continue
		}
		seen[m.Signer] = true
	}
	return len(seen) == 3
}
