package agent

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

const (
	maxSwitchCiphertext = 11 << 20
	maxTransitionBody   = 16 << 20
)

var (
	ErrTransitionUnavailable = errors.New("agent transition is unavailable")
	ErrTransitionConflict    = errors.New("agent transition conflicts with durable state")
)

// TransitionConfig pins one admitted switch and its local agent identities.
type TransitionConfig struct {
	Journal             *Journal
	OwnerKey            ed25519.PublicKey
	EnrollmentBody      []byte
	EnrollmentSignature []byte
	AgentID             string
	SigningKey          ed25519.PrivateKey
	AgeIdentity         string
	ReadClock           func() (Reading, error)
}

// Transitions serializes authenticated preparation and arm operations.
type Transitions struct {
	mu          sync.Mutex
	journal     *Journal
	enrollment  switchwire.AdmittedEnrollment
	enrollmentB []byte
	agentID     string
	signingKey  ed25519.PrivateKey
	ageIdentity *age.X25519Identity
	clock       func() (Reading, error)
}

type transitionRequest struct {
	Request    switchwire.Envelope `json:"request"`
	Ciphertext []byte              `json:"ciphertext"`
	Manifest   []byte              `json:"manifest"`
}

// NewTransitions verifies the pinned certificate and local agent keys.
func NewTransitions(config TransitionConfig) (*Transitions, error) {
	if config.Journal == nil || len(config.OwnerKey) != ed25519.PublicKeySize || len(config.SigningKey) != ed25519.PrivateKeySize || config.AgentID == "" {
		return nil, errors.New("agent transition configuration is invalid")
	}
	enrollment, err := switchwire.AdmitCertificate(config.EnrollmentBody, config.EnrollmentSignature, config.OwnerKey)
	if err != nil {
		return nil, fmt.Errorf("admit switch enrollment: %w", err)
	}
	var local *switchwire.Agent
	for i := range enrollment.Agents {
		if enrollment.Agents[i].ID == config.AgentID {
			local = &enrollment.Agents[i]
			break
		}
	}
	if local == nil || !bytes.Equal(config.SigningKey.Public().(ed25519.PublicKey), local.SigningKey[:]) {
		return nil, errors.New("local agent keys do not match enrollment")
	}
	identity, err := age.ParseX25519Identity(config.AgeIdentity)
	if err != nil || identity.Recipient().String() != local.Recipient {
		return nil, errors.New("local age identity does not match enrollment")
	}
	enrollmentBytes, err := encodeStoredEnrollment(config.EnrollmentBody, config.EnrollmentSignature)
	if err != nil {
		return nil, err
	}
	return &Transitions{
		journal: config.Journal, enrollment: enrollment, enrollmentB: enrollmentBytes,
		agentID: config.AgentID, signingKey: ed25519.PrivateKey(bytes.Clone(config.SigningKey)), ageIdentity: identity, clock: config.ReadClock,
	}, nil
}

// Prepare stores the ciphertext and this agent's decrypted share before its receipt.
func (t *Transitions) Prepare(request switchwire.Envelope, ciphertext, manifest []byte) (switchwire.Envelope, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	frame, entry, requestBytes, err := t.validatePrepare(request, ciphertext, manifest)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	current, err := t.journal.Snapshot()
	if err != nil {
		return switchwire.Envelope{}, fmt.Errorf("read agent authority: %w", err)
	}
	if current.Phase != 0 {
		return t.retryPrepare(current, frame, entry, requestBytes, ciphertext)
	}
	return t.acceptPrepare(current, frame, entry, requestBytes, ciphertext)
}

func (t *Transitions) retryPrepare(current State, frame switchwire.Frame, entry switchwire.Entry, requestBytes, ciphertext []byte) (switchwire.Envelope, error) {
	if current.Phase != switchwire.PhasePrepared || !bytes.Equal(current.AcceptedRequest, requestBytes) || !bytes.Equal(current.Ciphertext, ciphertext) || !bytes.Equal(current.EncryptedShare, entry.Data) || !bytes.Equal(current.Enrollment, t.enrollmentB) {
		return switchwire.Envelope{}, ErrTransitionConflict
	}
	if err := t.validateStoredPrepare(current, frame.State); err != nil {
		return switchwire.Envelope{}, err
	}
	if len(current.Receipt) != 0 {
		if err := t.validateReceipt(current.Receipt, frame.State, switchwire.PhasePrepared); err != nil {
			return switchwire.Envelope{}, err
		}
	}
	if err := t.checkClock(current); err != nil {
		return switchwire.Envelope{}, err
	}
	if len(current.Receipt) != 0 {
		return switchwire.ReadEnvelope(bytes.NewReader(current.Receipt))
	}
	return t.persistReceipt(current, frame.State, switchwire.PhasePrepared)
}

func (t *Transitions) acceptPrepare(current State, frame switchwire.Frame, entry switchwire.Entry, requestBytes, ciphertext []byte) (switchwire.Envelope, error) {
	if frame.State.Sequence != 1 {
		return switchwire.Envelope{}, ErrTransitionConflict
	}
	if err := t.checkClockForBoundary(frame, current); err != nil {
		return switchwire.Envelope{}, err
	}
	current = State{
		Phase: switchwire.PhasePrepared, AcceptedRequest: requestBytes, PrepareRequest: bytes.Clone(requestBytes),
		Enrollment: bytes.Clone(t.enrollmentB), Ciphertext: bytes.Clone(ciphertext),
		EncryptedShare: bytes.Clone(entry.Data),
	}
	if _, err := t.commit(current); err != nil {
		return switchwire.Envelope{}, err
	}
	return t.persistReceipt(current, frame.State, switchwire.PhasePrepared)
}

// Arm accepts one owner request with three matching prepared receipts.
func (t *Transitions) Arm(request switchwire.Envelope) (switchwire.Envelope, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	frame, err := switchwire.VerifyOwnerRequest(request, t.enrollment)
	if err != nil || frame.Operation != switchwire.OperationArm {
		return switchwire.Envelope{}, errors.New("owner arm request is invalid")
	}
	requestBytes, err := switchwire.MarshalEnvelope(request)
	if err != nil {
		return switchwire.Envelope{}, fmt.Errorf("encode arm request: %w", err)
	}
	current, err := t.journal.Snapshot()
	if err != nil {
		return switchwire.Envelope{}, fmt.Errorf("read agent authority: %w", err)
	}
	if current.Phase == switchwire.PhaseArmed {
		return t.retryArm(current, frame, requestBytes)
	}
	return t.acceptArm(current, frame, requestBytes)
}

func (t *Transitions) retryArm(current State, frame switchwire.Frame, requestBytes []byte) (switchwire.Envelope, error) {
	if !bytes.Equal(current.AcceptedRequest, requestBytes) || !bytes.Equal(current.Enrollment, t.enrollmentB) {
		return switchwire.Envelope{}, ErrTransitionConflict
	}
	if err := t.validateStoredPrepare(current, frame.State); err != nil {
		return switchwire.Envelope{}, err
	}
	if err := t.validateAcceptedArm(current.AcceptedRequest, frame.State); err != nil {
		return switchwire.Envelope{}, err
	}
	if len(current.Receipt) != 0 {
		if err := t.validateReceipt(current.Receipt, frame.State, switchwire.PhaseArmed); err != nil {
			return switchwire.Envelope{}, err
		}
	}
	if err := t.checkClock(current); err != nil {
		return switchwire.Envelope{}, err
	}
	if len(current.Receipt) != 0 {
		return switchwire.ReadEnvelope(bytes.NewReader(current.Receipt))
	}
	return t.persistReceipt(current, frame.State, switchwire.PhaseArmed)
}

func (t *Transitions) acceptArm(current State, frame switchwire.Frame, requestBytes []byte) (switchwire.Envelope, error) {
	if err := t.validatePreparedAuthority(current, frame.State); err != nil {
		return switchwire.Envelope{}, err
	}
	if err := t.validateArmReceipts(current.Receipt, frame); err != nil {
		return switchwire.Envelope{}, err
	}
	if err := t.checkClock(current); err != nil {
		return switchwire.Envelope{}, err
	}
	current.Phase = switchwire.PhaseArmed
	current.AcceptedRequest = requestBytes
	current.Receipt = nil
	if _, err := t.commit(current); err != nil {
		return switchwire.Envelope{}, err
	}
	return t.persistReceipt(current, frame.State, switchwire.PhaseArmed)
}

func (t *Transitions) validatePreparedAuthority(current State, state switchwire.State) error {
	if current.Phase != switchwire.PhasePrepared || !bytes.Equal(current.Enrollment, t.enrollmentB) {
		return ErrTransitionConflict
	}
	preparedRequest, err := switchwire.ReadEnvelope(bytes.NewReader(current.AcceptedRequest))
	if err != nil {
		return fmt.Errorf("decode prepared request: %w", err)
	}
	preparedFrame, err := switchwire.VerifyOwnerRequest(preparedRequest, t.enrollment)
	if err != nil || preparedFrame.Operation != switchwire.OperationPrepare || preparedFrame.State != state {
		return ErrTransitionConflict
	}
	if err := t.validateStoredPrepare(current, state); err != nil {
		return err
	}
	if len(current.Receipt) == 0 {
		return ErrTransitionConflict
	}
	return t.validateReceipt(current.Receipt, state, switchwire.PhasePrepared)
}

func (t *Transitions) validateArmReceipts(localData []byte, frame switchwire.Frame) error {
	localReceipt, err := switchwire.ReadEnvelope(bytes.NewReader(localData))
	if err != nil {
		return fmt.Errorf("decode prepared agent receipt: %w", err)
	}
	localReceiptIncluded := false
	for _, receipt := range armReceipts(frame) {
		if bytes.Equal(receipt.Body, localReceipt.Body) && bytes.Equal(receipt.Signature, localReceipt.Signature) {
			localReceiptIncluded = true
		}
	}
	if !localReceiptIncluded {
		return ErrTransitionConflict
	}
	if !switchwire.Confirm(frame.State, armReceipts(frame), t.enrollment, switchwire.PhasePrepared) {
		return errors.New("arm request lacks three prepared receipts")
	}
	return nil
}

// Handler exposes bounded JSON endpoints for initial preparation and arm.
func (t *Transitions) Handler() http.Handler {
	return http.HandlerFunc(t.serveTransition)
}

func (t *Transitions) serveTransition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pathID, operation, ok := parseTransitionPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	input, err := readTransitionRequest(w, r)
	if err != nil {
		status := http.StatusBadRequest
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, "request body is invalid", status)
		return
	}
	frame, err := switchwire.DecodeFrame(input.Request.Body)
	if err != nil || frame.State.Switch != pathID {
		http.Error(w, "switch request does not match path", http.StatusBadRequest)
		return
	}
	response, err := t.dispatchTransition(operation, input)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, ErrClock) || errors.Is(err, ErrTransitionUnavailable) {
			status = http.StatusServiceUnavailable
		}
		http.Error(w, "switch transition rejected", status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func parseTransitionPath(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 5 || parts[0] != "agent" || parts[1] != "v1" || parts[2] != "switches" || parts[3] == "" {
		return "", "", false
	}
	if parts[4] != "prepare" && parts[4] != "arm" {
		return "", "", false
	}
	return parts[3], parts[4], true
}

func readTransitionRequest(w http.ResponseWriter, r *http.Request) (transitionRequest, error) {
	if r.Body == nil {
		return transitionRequest{}, errors.New("request body is missing")
	}
	bounded := http.MaxBytesReader(w, r.Body, maxTransitionBody)
	data, readErr := io.ReadAll(bounded)
	closeErr := r.Body.Close()
	if readErr != nil {
		return transitionRequest{}, readErr
	}
	if closeErr != nil {
		return transitionRequest{}, fmt.Errorf("close transition request body: %w", closeErr)
	}
	var input transitionRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return transitionRequest{}, fmt.Errorf("decode transition request: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return transitionRequest{}, errors.New("request body has trailing data")
	}
	return input, nil
}

func (t *Transitions) dispatchTransition(operation string, request transitionRequest) (switchwire.Envelope, error) {
	if operation == "prepare" {
		return t.Prepare(request.Request, request.Ciphertext, request.Manifest)
	}
	return t.Arm(request.Request)
}

func (t *Transitions) validatePrepare(request switchwire.Envelope, ciphertext, manifest []byte) (switchwire.Frame, switchwire.Entry, []byte, error) {
	frame, err := switchwire.VerifyOwnerRequest(request, t.enrollment)
	if err != nil || frame.Operation != switchwire.OperationPrepare {
		return switchwire.Frame{}, switchwire.Entry{}, nil, errors.New("owner prepare request is invalid")
	}
	if len(ciphertext) == 0 || len(ciphertext) > maxSwitchCiphertext || len(manifest) != 0 || switchcontent.ValidateBinding(ciphertext, frame.State.Key, frame.State.Content) != nil {
		return switchwire.Frame{}, switchwire.Entry{}, nil, errors.New("prepared content does not match the signed digest")
	}
	var own *switchwire.Entry
	for i := range frame.Entries {
		if frame.Entries[i].ID == t.agentID {
			own = &frame.Entries[i]
			break
		}
	}
	if own == nil {
		return switchwire.Frame{}, switchwire.Entry{}, nil, errors.New("prepare request omits this agent")
	}
	if err := validateEncryptedShare(t.ageIdentity, own.Data); err != nil {
		return switchwire.Frame{}, switchwire.Entry{}, nil, err
	}
	requestBytes, err := switchwire.MarshalEnvelope(request)
	if err != nil {
		return switchwire.Frame{}, switchwire.Entry{}, nil, fmt.Errorf("encode prepare request: %w", err)
	}
	return frame, *own, requestBytes, nil
}

func (t *Transitions) checkClock(current State) error {
	request, err := switchwire.ReadEnvelope(bytes.NewReader(current.AcceptedRequest))
	if err != nil {
		return fmt.Errorf("decode accepted agent request: %w", err)
	}
	frame, err := switchwire.DecodeFrame(request.Body)
	if err != nil {
		return fmt.Errorf("decode accepted agent frame: %w", err)
	}
	return t.checkClockForBoundary(frame, current)
}

func (t *Transitions) checkClockForBoundary(frame switchwire.Frame, current State) error {
	if current.PossibleDisclosure || current.UnsafeClock {
		return ErrClock
	}
	if t.clock == nil {
		return ErrClock
	}
	reading, err := t.clock()
	if err != nil {
		current.UnsafeClock = true
		if _, commitErr := t.commit(current); commitErr != nil {
			return errors.Join(fmt.Errorf("assess agent clock: %w", ErrClock), fmt.Errorf("persist unsafe clock latch: %w", commitErr))
		}
		return fmt.Errorf("assess agent clock: %w", ErrClock)
	}
	deadline, grace := frame.State.Deadline, frame.State.Grace
	// The frame decoder rejects negative deadlines before this checked cast.
	if deadline < 0 || grace > uint64(math.MaxInt64-deadline) { // #nosec G115 -- The nonnegative deadline is at most MaxInt64.
		return ErrClock
	}
	if reading.LowerUTC() >= deadline+int64(grace) { // #nosec G115 -- The prior check bounds grace to MaxInt64 minus deadline.
		return ErrClock
	}
	interval := t.enrollment.IntervalSeconds()
	if interval > uint64(math.MaxInt64) || deadline < int64(interval) {
		return ErrClock
	}
	issued := deadline - int64(interval)
	upper := reading.LowerUTC()
	if upper > math.MaxInt64-2*CandidateEpsilonSeconds {
		upper = math.MaxInt64
	} else {
		upper += 2 * CandidateEpsilonSeconds
	}
	if issued > upper {
		return ErrClock
	}
	return nil
}

func (t *Transitions) commit(state State) (State, error) {
	current, err := t.journal.Snapshot()
	if err != nil {
		return State{}, fmt.Errorf("read agent authority before commit: %w", err)
	}
	if current.Generation == math.MaxUint64 {
		return State{}, ErrGeneration
	}
	state.Generation = current.Generation + 1
	committed, err := t.journal.Commit(state)
	if err != nil {
		return State{}, fmt.Errorf("persist agent transition: %w", err)
	}
	return committed, nil
}

func (t *Transitions) persistReceipt(state State, wireState switchwire.State, phase byte) (switchwire.Envelope, error) {
	receipt, err := switchwire.SignFrame(switchwire.Frame{
		Operation: switchwire.OperationReceipt, Signer: t.agentID, State: wireState,
		Durable: wireState.Sequence, Phase: phase,
	}, t.signingKey)
	if err != nil {
		return switchwire.Envelope{}, fmt.Errorf("sign durable agent receipt: %w", err)
	}
	encoded, err := switchwire.MarshalEnvelope(receipt)
	if err != nil {
		return switchwire.Envelope{}, fmt.Errorf("encode durable agent receipt: %w", err)
	}
	state.Receipt = encoded
	if _, err := t.commit(state); err != nil {
		return switchwire.Envelope{}, err
	}
	return receipt, nil
}

func (t *Transitions) validateStoredPrepare(state State, expected switchwire.State) error {
	if err := t.validateStoredContent(state, expected); err != nil {
		return err
	}
	requestBytes := state.PrepareRequest
	if len(requestBytes) == 0 {
		requestBytes = state.AcceptedRequest
	}
	request, err := switchwire.ReadEnvelope(bytes.NewReader(requestBytes))
	if err != nil {
		return fmt.Errorf("decode stored prepare request: %w", err)
	}
	frame, err := switchwire.VerifyOwnerRequest(request, t.enrollment)
	if err != nil || frame.Operation != switchwire.OperationPrepare || frame.State != expected {
		return ErrTransitionConflict
	}
	for _, entry := range frame.Entries {
		if entry.ID == t.agentID {
			if !bytes.Equal(entry.Data, state.EncryptedShare) {
				return ErrTransitionConflict
			}
			return validateEncryptedShare(t.ageIdentity, state.EncryptedShare)
		}
	}
	return ErrTransitionConflict
}

func (t *Transitions) validateStoredContent(state State, expected switchwire.State) error {
	if switchcontent.ValidateBinding(state.Ciphertext, expected.Key, expected.Content) != nil || !bytes.Equal(state.Enrollment, t.enrollmentB) {
		return ErrTransitionConflict
	}
	return validateEncryptedShare(t.ageIdentity, state.EncryptedShare)
}

func (t *Transitions) validateAcceptedArm(data []byte, expected switchwire.State) error {
	request, err := switchwire.ReadEnvelope(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode stored arm request: %w", err)
	}
	frame, err := switchwire.VerifyOwnerRequest(request, t.enrollment)
	if err != nil || frame.Operation != switchwire.OperationArm || frame.State != expected {
		return ErrTransitionConflict
	}
	if !switchwire.Confirm(expected, armReceipts(frame), t.enrollment, switchwire.PhasePrepared) {
		return ErrTransitionConflict
	}
	return nil
}

func (t *Transitions) validateReceipt(data []byte, state switchwire.State, phase byte) error {
	receipt, err := switchwire.ReadEnvelope(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode stored agent receipt: %w", err)
	}
	public := t.signingKey.Public().(ed25519.PublicKey)
	frame, err := switchwire.VerifyEnvelope(receipt, map[string]ed25519.PublicKey{t.agentID: public})
	if err != nil || frame.Operation != switchwire.OperationReceipt || frame.State != state || frame.Phase != phase || frame.Durable != state.Sequence || frame.Released {
		return ErrTransitionConflict
	}
	return nil
}

func armReceipts(frame switchwire.Frame) []switchwire.Envelope {
	receipts := make([]switchwire.Envelope, 0, len(frame.Entries))
	for _, entry := range frame.Entries {
		receipt, err := switchwire.ReadEnvelope(bytes.NewReader(entry.Data))
		if err == nil {
			receipts = append(receipts, receipt)
		}
	}
	return receipts
}

func validateEncryptedShare(identity *age.X25519Identity, ciphertext []byte) error {
	share, err := switchcontent.DecryptShare(ciphertext, identity)
	if err != nil {
		return errors.New("prepared share does not match this agent")
	}
	clear(share)
	return nil
}

func encodeStoredEnrollment(body, signature []byte) ([]byte, error) {
	if len(body) == 0 || len(body) > switchwire.MaxCertificate || uint64(len(body)) > uint64(^uint32(0)) || len(signature) != ed25519.SignatureSize {
		return nil, errors.New("stored enrollment has an invalid size")
	}
	out := make([]byte, 4+len(body)+len(signature))
	binary.BigEndian.PutUint32(out[:4], uint32(len(body))) // #nosec G115 -- The certificate limit bounds this length to 8192 bytes.
	copy(out[4:], body)
	copy(out[4+len(body):], signature)
	return out, nil
}
