// Package switchwire encodes and verifies the version two switch protocol.
package switchwire

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"

	"filippo.io/age"
)

// Version identifies the version two protocol.
const (
	Version                 uint32 = 2
	OperationPrepare               = "prepare"
	OperationArm                   = "arm"
	OperationCheckin               = "checkin"
	OperationStatusRequest         = "status-request"
	OperationStatusResponse        = "status-response"
	OperationReceipt               = "receipt"
	OperationRelease               = "release"
	OperationCancel                = "cancel"
	PhaseDraft              byte   = 0
	PhasePrepared           byte   = 1
	PhaseArmed              byte   = 2
	PhaseStopped            byte   = 3
	MaxBody                        = 16384
	MaxEnvelope                    = MaxBody + ed25519.SignatureSize + 4
	MaxCertificate                 = 8192
)

const (
	frameDomain       = "epochalypse/switch/research-v2/"
	certificateDomain = "epochalypse/switch/enrollment-v2/"
)

var errInvalid = errors.New("invalid switch wire data")

// State binds a protocol frame to one durable switch state.
type State struct {
	Switch                   string
	Version                  uint32
	Sequence                 uint64
	Deadline                 int64
	Grace                    uint64
	Content, Key, Enrollment [32]byte
}

// Entry carries one operation-specific opaque payload.
type Entry struct {
	ID   string
	Key  [32]byte
	Data []byte
}

// Frame is the typed content of a signed protocol body.
type Frame struct {
	Operation, Signer string
	State             State
	Nonce             [32]byte
	Durable           uint64
	Phase             byte
	Released          bool
	Entries           []Entry
}

// Envelope carries one signed frame.
type Envelope struct{ Body, Signature []byte }

// Agent binds one signer to one age recipient and recovery endpoint.
type Agent struct {
	ID                  string
	SigningKey          [32]byte
	Recipient, Endpoint string
}

// Certificate binds the owner, content, timing policy, and three agents.
type Certificate struct {
	OwnerKey, Key, Content [32]byte
	Interval, Grace        uint64
	Version                uint32
	Agents                 [3]Agent
}

// AdmittedEnrollment holds a verified immutable enrollment snapshot.
type AdmittedEnrollment struct {
	Digest   [32]byte
	Agents   [3]Agent
	OwnerKey [32]byte
	digest   [32]byte
	agents   [3]Agent
	ownerKey [32]byte
	key      [32]byte
	content  [32]byte
	grace    uint64
	interval uint64
	valid    bool
}

// EncodeFrame returns the canonical signed body for a frame.
func EncodeFrame(f Frame) ([]byte, error) {
	if err := validateFrame(f); err != nil {
		return nil, err
	}
	b := new(bytes.Buffer)
	b.WriteString(frameDomain)
	b.WriteString(f.Operation)
	b.WriteByte(0)
	putField(b, []byte(f.State.Switch))
	_ = binary.Write(b, binary.BigEndian, f.State.Version)
	_ = binary.Write(b, binary.BigEndian, f.State.Sequence)
	_ = binary.Write(b, binary.BigEndian, f.State.Deadline)
	_ = binary.Write(b, binary.BigEndian, f.State.Grace)
	b.Write(f.State.Content[:])
	b.Write(f.State.Key[:])
	b.Write(f.State.Enrollment[:])
	putField(b, []byte(f.Signer))
	b.Write(f.Nonce[:])
	_ = binary.Write(b, binary.BigEndian, f.Durable)
	b.WriteByte(f.Phase)
	flag := byte(0)
	if f.Released {
		flag = 1
	}
	b.WriteByte(flag)
	entryCounts := [4][4]byte{{}, {0, 0, 0, 1}, {0, 0, 0, 2}, {0, 0, 0, 3}}
	b.Write(entryCounts[len(f.Entries)][:])
	for _, e := range f.Entries {
		putField(b, []byte(e.ID))
		b.Write(e.Key[:])
		putField(b, e.Data)
	}
	if b.Len() > MaxBody {
		return nil, fmt.Errorf("%w: frame exceeds limit", errInvalid)
	}
	return b.Bytes(), nil
}

// DecodeFrame returns a validated frame from one canonical body.
func DecodeFrame(body []byte) (Frame, error) {
	var f Frame
	if len(body) > MaxBody || !bytes.HasPrefix(body, []byte(frameDomain)) {
		return f, fmt.Errorf("%w: frame domain or size", errInvalid)
	}
	rest := body[len(frameDomain):]
	separator := bytes.IndexByte(rest, 0)
	if separator < 1 || separator > 16 {
		return f, fmt.Errorf("%w: operation", errInvalid)
	}
	f.Operation = string(rest[:separator])
	d := decoder{r: bytes.NewReader(rest[separator+1:])}
	f.State.Switch = string(d.field(128))
	d.fixed(&f.State.Version)
	d.fixed(&f.State.Sequence)
	d.fixed(&f.State.Deadline)
	d.fixed(&f.State.Grace)
	d.fixed(&f.State.Content)
	d.fixed(&f.State.Key)
	d.fixed(&f.State.Enrollment)
	f.Signer = string(d.field(128))
	d.fixed(&f.Nonce)
	d.fixed(&f.Durable)
	d.fixed(&f.Phase)
	var flag byte
	d.fixed(&flag)
	f.Released = flag == 1
	var count uint32
	d.fixed(&count)
	if d.err != nil || count > 3 || flag > 1 {
		return Frame{}, fmt.Errorf("%w: frame flags or count", errInvalid)
	}
	for range count {
		e := Entry{ID: string(d.field(128))}
		d.fixed(&e.Key)
		e.Data = d.field(4096)
		f.Entries = append(f.Entries, e)
	}
	if d.err != nil || d.r.Len() != 0 {
		return Frame{}, fmt.Errorf("%w: frame fields or trailing bytes", errInvalid)
	}
	if err := validateFrame(f); err != nil {
		return Frame{}, err
	}
	canonical, err := EncodeFrame(f)
	if err != nil || !bytes.Equal(canonical, body) {
		return Frame{}, fmt.Errorf("%w: noncanonical frame", errInvalid)
	}
	return f, nil
}

// SignFrame signs the canonical bytes with an Ed25519 private key.
func SignFrame(f Frame, key ed25519.PrivateKey) (Envelope, error) {
	if len(key) != ed25519.PrivateKeySize {
		return Envelope{}, fmt.Errorf("%w: private key length", errInvalid)
	}
	body, err := EncodeFrame(f)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Body: body, Signature: ed25519.Sign(key, body)}, nil
}

// VerifyEnvelope authenticates a frame against its signer's enrolled key.
func VerifyEnvelope(e Envelope, keys map[string]ed25519.PublicKey) (Frame, error) {
	f, err := DecodeFrame(e.Body)
	if err != nil {
		return Frame{}, err
	}
	key := keys[f.Signer]
	if len(key) != ed25519.PublicKeySize || len(e.Signature) != ed25519.SignatureSize || !ed25519.Verify(key, e.Body, e.Signature) {
		return Frame{}, fmt.Errorf("%w: signature or signer", errInvalid)
	}
	return f, nil
}

// MarshalEnvelope writes a four-byte body length, body, and signature.
func MarshalEnvelope(e Envelope) ([]byte, error) {
	if len(e.Body) == 0 || len(e.Body) > MaxBody || len(e.Signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: envelope size", errInvalid)
	}
	b := make([]byte, 4+len(e.Body)+ed25519.SignatureSize)
	if err := writeLength(b[:4], len(e.Body)); err != nil {
		return nil, err
	}
	copy(b[4:], e.Body)
	copy(b[4+len(e.Body):], e.Signature)
	return b, nil
}

// ReadEnvelope reads one bounded envelope and rejects trailing stream bytes.
func ReadEnvelope(r io.Reader) (Envelope, error) {
	var out Envelope
	b, err := io.ReadAll(io.LimitReader(r, MaxEnvelope+1))
	if err != nil {
		return out, fmt.Errorf("read switch envelope: %w", err)
	}
	if len(b) > MaxEnvelope || len(b) < 4+ed25519.SignatureSize {
		return out, fmt.Errorf("%w: envelope size", errInvalid)
	}
	n := binary.BigEndian.Uint32(b[:4])
	if n == 0 || n > MaxBody || uint64(len(b)) != uint64(n)+4+ed25519.SignatureSize {
		return out, fmt.Errorf("%w: envelope length", errInvalid)
	}
	out.Body = bytes.Clone(b[4 : 4+int(n)])
	out.Signature = bytes.Clone(b[4+int(n):])
	return out, nil
}

// EncodeCertificate returns the canonical enrollment certificate bytes.
func EncodeCertificate(c Certificate) ([]byte, error) {
	if err := validateCertificate(c); err != nil {
		return nil, err
	}
	b := new(bytes.Buffer)
	b.WriteString(certificateDomain)
	b.WriteByte(0)
	b.Write(c.OwnerKey[:])
	b.Write(c.Key[:])
	b.Write(c.Content[:])
	_ = binary.Write(b, binary.BigEndian, c.Interval)
	_ = binary.Write(b, binary.BigEndian, c.Grace)
	_ = binary.Write(b, binary.BigEndian, c.Version)
	b.Write([]byte{0, 0, 0, 3})
	for _, a := range c.Agents {
		putField(b, []byte(a.ID))
		b.Write(a.SigningKey[:])
		putField(b, []byte(a.Recipient))
		putField(b, []byte(a.Endpoint))
	}
	if b.Len() > MaxCertificate {
		return nil, fmt.Errorf("%w: certificate exceeds limit", errInvalid)
	}
	return b.Bytes(), nil
}

// AdmitCertificate verifies a canonical owner-signed certificate and copies its agents.
func AdmitCertificate(body, signature []byte, ownerKey ed25519.PublicKey) (AdmittedEnrollment, error) {
	if len(body) == 0 || len(body) > MaxCertificate || len(signature) != ed25519.SignatureSize || len(ownerKey) != ed25519.PublicKeySize {
		return AdmittedEnrollment{}, fmt.Errorf("%w: certificate size or key", errInvalid)
	}
	c, err := decodeCertificate(body)
	if err != nil {
		return AdmittedEnrollment{}, err
	}
	if !bytes.Equal(c.OwnerKey[:], ownerKey) || !ed25519.Verify(ownerKey, body, signature) {
		return AdmittedEnrollment{}, fmt.Errorf("%w: owner signature", errInvalid)
	}
	digest := sha256.Sum256(body)
	return AdmittedEnrollment{Digest: digest, Agents: c.Agents, OwnerKey: c.OwnerKey, digest: digest, agents: c.Agents, ownerKey: c.OwnerKey, key: c.Key, content: c.Content, grace: c.Grace, interval: c.Interval, valid: true}, nil
}

// IntervalSeconds returns the authenticated interval from the admitted certificate.
func (e AdmittedEnrollment) IntervalSeconds() uint64 { return e.interval }

// VerifyOwnerRequest authenticates and admits an owner operation against enrollment.
func VerifyOwnerRequest(e Envelope, enrollment AdmittedEnrollment) (Frame, error) {
	if !enrollment.valid {
		return Frame{}, fmt.Errorf("%w: enrollment is not admitted", errInvalid)
	}
	key := enrollment.ownerKey
	keys := map[string]ed25519.PublicKey{"owner": key[:]}
	f, err := VerifyEnvelope(e, keys)
	if err != nil {
		return Frame{}, err
	}
	if err := validateOwnerState(f, enrollment); err != nil {
		return Frame{}, err
	}
	if err := validateOwnerOperation(f, enrollment); err != nil {
		return Frame{}, err
	}
	return f, nil
}

func validateOwnerState(f Frame, enrollment AdmittedEnrollment) error {
	validState := f.Signer == "owner" && f.State.Enrollment == enrollment.digest && f.State.Key == enrollment.key && f.State.Content == enrollment.content && f.State.Grace == enrollment.grace
	validEnrollment := enrollment.Digest == enrollment.digest && enrollment.OwnerKey == enrollment.ownerKey && enrollment.Agents == enrollment.agents
	if !validState || !validEnrollment {
		return fmt.Errorf("%w: owner state enrollment", errInvalid)
	}
	return nil
}

func validateOwnerOperation(f Frame, enrollment AdmittedEnrollment) error {
	switch f.Operation {
	case OperationPrepare:
		return validatePrepare(f, enrollment)
	case OperationArm:
		return validateArm(f, enrollment)
	case OperationCheckin, OperationStatusRequest, OperationCancel:
		if len(f.Entries) != 0 {
			return fmt.Errorf("%w: owner entries", errInvalid)
		}
		return nil
	default:
		return fmt.Errorf("%w: owner operation", errInvalid)
	}
}

func validatePrepare(f Frame, enrollment AdmittedEnrollment) error {
	if f.Phase != PhasePrepared || f.Released || len(f.Entries) != 3 {
		return fmt.Errorf("%w: prepare phase or entries", errInvalid)
	}
	for i, a := range enrollment.agents {
		r, err := age.ParseX25519Recipient(a.Recipient)
		if err != nil || r.String() != a.Recipient {
			return fmt.Errorf("%w: enrolled recipient", errInvalid)
		}
		entry := f.Entries[i]
		if entry.ID != a.ID || entry.Key != sha256.Sum256([]byte(r.String())) {
			return fmt.Errorf("%w: prepared recipient binding", errInvalid)
		}
	}
	return nil
}

func validateArm(f Frame, enrollment AdmittedEnrollment) error {
	if f.Phase != PhaseArmed || f.Released || len(f.Entries) != 3 {
		return fmt.Errorf("%w: arm phase or entries", errInvalid)
	}
	keys := enrolledKeys(enrollment)
	seen := make(map[string]bool, 3)
	for _, entry := range f.Entries {
		if err := validateArmEntry(f, entry, enrollment, keys, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateArmEntry(f Frame, entry Entry, enrollment AdmittedEnrollment, keys map[string]ed25519.PublicKey, seen map[string]bool) error {
	a, ok := agentByID(enrollment, entry.ID)
	if !ok || seen[entry.ID] || entry.Key != a.SigningKey {
		return fmt.Errorf("%w: arm receipt identity", errInvalid)
	}
	seen[entry.ID] = true
	nested, err := ReadEnvelope(bytes.NewReader(entry.Data))
	if err != nil {
		return err
	}
	receipt, err := VerifyEnvelope(nested, keys)
	if err != nil || receipt.Operation != OperationReceipt || receipt.State != f.State || receipt.Phase != PhasePrepared || receipt.Released || receipt.Durable != f.State.Sequence || receipt.Signer != entry.ID {
		return fmt.Errorf("%w: prepared receipt evidence", errInvalid)
	}
	return nil
}

// Confirm requires one authenticated receipt from each enrolled agent.
func Confirm(state State, receipts []Envelope, enrollment AdmittedEnrollment, phase byte) bool {
	if !enrollment.valid || !stateMatchesEnrollment(state, enrollment) {
		return false
	}
	seen := map[string]bool{}
	keys := enrolledKeys(enrollment)
	for _, e := range receipts {
		f, err := VerifyEnvelope(e, keys)
		if err == nil && f.Operation == OperationReceipt && f.State == state && f.Phase == phase && !f.Released && f.Durable == state.Sequence {
			seen[f.Signer] = true
		}
	}
	return len(seen) == 3
}

func stateMatchesEnrollment(state State, enrollment AdmittedEnrollment) bool {
	return state.Enrollment == enrollment.digest && state.Key == enrollment.key && state.Content == enrollment.content && state.Grace == enrollment.grace && enrollment.Digest == enrollment.digest && enrollment.OwnerKey == enrollment.ownerKey && enrollment.Agents == enrollment.agents
}

func decodeCertificate(body []byte) (Certificate, error) {
	var c Certificate
	if len(body) > MaxCertificate || !bytes.HasPrefix(body, []byte(certificateDomain+"\x00")) {
		return c, fmt.Errorf("%w: certificate domain or size", errInvalid)
	}
	d := decoder{r: bytes.NewReader(body[len(certificateDomain)+1:])}
	d.fixed(&c.OwnerKey)
	d.fixed(&c.Key)
	d.fixed(&c.Content)
	d.fixed(&c.Interval)
	d.fixed(&c.Grace)
	d.fixed(&c.Version)
	var count uint32
	d.fixed(&count)
	if d.err != nil || count != 3 {
		return c, fmt.Errorf("%w: certificate count", errInvalid)
	}
	for i := range c.Agents {
		a := &c.Agents[i]
		a.ID = string(d.field(128))
		d.fixed(&a.SigningKey)
		a.Recipient = string(d.field(128))
		a.Endpoint = string(d.field(2048))
	}
	if d.err != nil || d.r.Len() != 0 {
		return c, fmt.Errorf("%w: certificate fields", errInvalid)
	}
	canonical, err := EncodeCertificate(c)
	if err != nil || !bytes.Equal(canonical, body) {
		return Certificate{}, fmt.Errorf("%w: noncanonical certificate", errInvalid)
	}
	return c, nil
}

func validateCertificate(c Certificate) error {
	if c.OwnerKey == [32]byte{} || c.Key == [32]byte{} || c.Content == [32]byte{} || c.Version != Version || c.Interval < 3600 || c.Interval > 31536000 || c.Grace > 2592000 {
		return fmt.Errorf("%w: certificate values", errInvalid)
	}
	return validateAgents(c.Agents)
}

func validateAgents(agents [3]Agent) error {
	seenIDs := map[string]bool{}
	seenKeys := map[[32]byte]bool{}
	seenRecipients := map[string]bool{}
	previous := ""
	for _, a := range agents {
		if !validID(a.ID) || a.ID == "owner" || previous >= a.ID || seenIDs[a.ID] || a.SigningKey == [32]byte{} || seenKeys[a.SigningKey] {
			return fmt.Errorf("%w: enrolled identity or signing key", errInvalid)
		}
		r, err := age.ParseX25519Recipient(a.Recipient)
		if err != nil || r.String() != a.Recipient || len(a.Recipient) > 128 || seenRecipients[a.Recipient] {
			return fmt.Errorf("%w: enrolled age recipient", errInvalid)
		}
		if !validEndpoint(a.Endpoint) {
			return fmt.Errorf("%w: recovery endpoint", errInvalid)
		}
		seenIDs[a.ID] = true
		seenKeys[a.SigningKey] = true
		seenRecipients[a.Recipient] = true
		previous = a.ID
	}
	return nil
}

func validEndpoint(s string) bool {
	if len(s) == 0 || len(s) > 2048 {
		return false
	}
	for _, b := range []byte(s) {
		if b < 0x21 || b > 0x7e {
			return false
		}
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == "" && !strings.Contains(s, "#") && u.Opaque == ""
}

func validateFrame(f Frame) error {
	if !validID(f.Signer) || !validID(f.State.Switch) || f.State.Version != Version || f.State.Sequence == 0 || f.State.Deadline < 0 || f.State.Grace > math.MaxInt64-uint64(f.State.Deadline) || f.Phase > PhaseStopped || len(f.Entries) > 3 {
		return fmt.Errorf("%w: frame state", errInvalid)
	}
	if err := validateOperation(f); err != nil {
		return err
	}
	return validateSignerAndEntries(f)
}

func validateOperation(f Frame) error {
	owner := false
	switch f.Operation {
	case OperationPrepare, OperationArm:
		owner = true
		if len(f.Entries) != 3 {
			return fmt.Errorf("%w: operation entries", errInvalid)
		}
	case OperationCheckin, OperationStatusRequest, OperationCancel:
		owner = true
		if len(f.Entries) != 0 {
			return fmt.Errorf("%w: operation entries", errInvalid)
		}
	case OperationReceipt, OperationStatusResponse:
		if len(f.Entries) != 0 || f.Durable != f.State.Sequence {
			return fmt.Errorf("%w: receipt sequence or entries", errInvalid)
		}
	case OperationRelease:
		if !f.Released || f.Durable != f.State.Sequence || len(f.Entries) != 1 || f.Entries[0].ID != f.Signer || len(f.Entries[0].Data) != 33 {
			return fmt.Errorf("%w: release evidence", errInvalid)
		}
	default:
		return fmt.Errorf("%w: operation", errInvalid)
	}
	return validateOperationRole(f, owner)
}

func validateOperationRole(f Frame, owner bool) error {
	if owner != (f.Signer == "owner") {
		return fmt.Errorf("%w: signer role", errInvalid)
	}
	return nil
}

func validateSignerAndEntries(f Frame) error {
	for i, e := range f.Entries {
		if !validID(e.ID) || len(e.Data) == 0 || len(e.Data) > 4096 || (i > 0 && f.Entries[i-1].ID >= e.ID) {
			return fmt.Errorf("%w: entry order or size", errInvalid)
		}
	}
	return nil
}

func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range []byte(s) {
		if !isIDCharacter(c) {
			return false
		}
	}
	return true
}

func isIDCharacter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}

func putField(b *bytes.Buffer, value []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value))) // #nosec G115 -- Frame and certificate validation bound each field before encoding.
	b.Write(length[:])
	b.Write(value)
}

func writeLength(out []byte, n int) error {
	if n < 0 || n > MaxBody {
		return fmt.Errorf("%w: length exceeds limit", errInvalid)
	}
	out[0] = byte((n >> 24) & 0xff)
	out[1] = byte((n >> 16) & 0xff)
	out[2] = byte((n >> 8) & 0xff)
	out[3] = byte(n & 0xff)
	return nil
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
		d.err = errors.New("field length exceeds limit")
		return nil
	}
	b := make([]byte, int(n))
	_, d.err = io.ReadFull(d.r, b)
	return b
}

func enrolledKeys(e AdmittedEnrollment) map[string]ed25519.PublicKey {
	out := make(map[string]ed25519.PublicKey, 3)
	for _, a := range e.agents {
		out[a.ID] = a.SigningKey[:]
	}
	return out
}
func agentByID(e AdmittedEnrollment, id string) (Agent, bool) {
	for _, a := range e.agents {
		if a.ID == id {
			return a, true
		}
	}
	return Agent{}, false
}
