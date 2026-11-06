package switchclient

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

// Renewer sends owner-signed requests directly to enrolled agents.
type Renewer interface {
	Checkin(context.Context, switchwire.Agent, switchwire.Envelope) (switchwire.Envelope, error)
}

// AgentEvidence reports one independently authenticated durable receipt.
type AgentEvidence struct {
	ID      string               `json:"id"`
	State   string               `json:"state"`
	Receipt *switchwire.Envelope `json:"receipt,omitempty"`
}

// Renewal reports confirmed authority separately from already-issued intent.
type Renewal struct {
	Evidence  switches.RenewalEvidence       `json:"evidence"`
	Confirmed bool                           `json:"confirmed"`
	B0        int64                          `json:"b0"`
	Bmax      int64                          `json:"bmax"`
	Deadline  int64                          `json:"deadline"`
	Grace     uint64                         `json:"grace_seconds"`
	Allowance string                         `json:"allowance_assumptions"`
	Request   switchwire.Envelope            `json:"request"`
	Receipts  map[string]switchwire.Envelope `json:"receipts"`
	Agents    []AgentEvidence                `json:"agents"`
}

const renewalAssumptions = "B0 is the earliest possible disclosure boundary. Missing receipts leave an uncertain outcome. Release can precede the proposed boundary or follow B0. Default uncertainty can approach 24 hours. E is separate: candidate E=34 seconds requires reviewed clock and publication premises; actual hosts have no numeric acceptance. Main loss plus one outage requires two reachable, honest agents with intact durable state."

// InitializeRenewal retains three authenticated arm receipts before renewal can begin.
func InitializeRenewal(path string, p Plan, key ed25519.PrivateKey, request switchwire.Envelope, receipts map[string]switchwire.Envelope) error {
	e, initial, err := validatePlan(p, key)
	if err != nil {
		return err
	}
	f, err := switchwire.VerifyOwnerRequest(request, e)
	if err != nil || f.Operation != switchwire.OperationArm || f.State != initial.State || !switchwire.Confirm(f.State, orderedReceipts(e, receipts), e, switchwire.PhaseArmed) {
		return errors.New("renewal requires three matching durable arm receipts")
	}
	lock, err := lockRenewal(path)
	if err != nil {
		return err
	}
	defer unlockRenewal(lock)
	if err := persistEnvelope(requestPath(path, 1), request); err != nil {
		return err
	}
	for _, a := range e.Agents {
		if err := persistEnvelope(receiptPath(path, 1, a.ID), receipts[a.ID]); err != nil {
			return err
		}
	}
	return nil
}

// Renew journals signed intent before any send and resumes incomplete intent exactly.
func Renew(ctx context.Context, path string, p Plan, key ed25519.PrivateKey, transport Renewer, issued time.Time) (Renewal, error) {
	e, _, err := validatePlan(p, key)
	if err != nil {
		return Renewal{}, err
	}
	if transport == nil {
		return Renewal{}, errors.New("renewal transport is required")
	}
	lock, err := lockRenewal(path)
	if err != nil {
		return Renewal{}, err
	}
	defer unlockRenewal(lock)
	r, frame, err := readRenewal(path, p, e)
	if err != nil {
		return r, err
	}
	if r.Confirmed {
		if err := persistConfirmedRenewal(path, frame.State.Sequence, r, e); err != nil {
			r.Confirmed = false
			return r, err
		}
		frame, err = nextRenewal(frame, e, issued)
		if err != nil {
			return r, err
		}
		r.Request, err = switchwire.SignFrame(frame, key)
		if err != nil {
			return r, err
		}
		r.Receipts = map[string]switchwire.Envelope{}
	}
	// A previous sync can fail while exact bytes remain visible after restart.
	if err := persistEnvelope(requestPath(path, frame.State.Sequence), r.Request); err != nil {
		return r, err
	}
	r = renewalView(r, frame, r.Request, r.Receipts, e)
	for _, a := range e.Agents {
		if err := ctx.Err(); err != nil {
			return renewalView(r, frame, r.Request, r.Receipts, e), err
		}
		receipt, err := transport.Checkin(ctx, a, r.Request)
		if err != nil || !validRenewalReceipt(receipt, frame.State, e, a.ID) {
			continue
		}
		if err := persistEnvelope(receiptPath(path, frame.State.Sequence, a.ID), receipt); err != nil {
			return renewalView(r, frame, r.Request, r.Receipts, e), err
		}
		r.Receipts[a.ID] = receipt
	}
	return renewalView(r, frame, r.Request, r.Receipts, e), nil
}

func persistConfirmedRenewal(path string, sequence uint64, r Renewal, e switchwire.AdmittedEnrollment) error {
	if err := persistEnvelope(requestPath(path, sequence), r.Request); err != nil {
		return err
	}
	for _, a := range e.Agents {
		if err := persistEnvelope(receiptPath(path, sequence, a.ID), r.Receipts[a.ID]); err != nil {
			return err
		}
	}
	return nil
}

// RenewalStatus validates retained intent and each receipt without a main-server request.
func RenewalStatus(path string, p Plan, owner ed25519.PublicKey) (Renewal, error) {
	e, err := switchwire.AdmitCertificate(p.Certificate, p.CertificateSignature, owner)
	if err != nil {
		return Renewal{}, err
	}
	lock, err := lockRenewal(path)
	if err != nil {
		return Renewal{}, err
	}
	defer unlockRenewal(lock)
	r, _, err := readRenewal(path, p, e)
	return r, err
}

func nextRenewal(prior switchwire.Frame, e switchwire.AdmittedEnrollment, issued time.Time) (switchwire.Frame, error) {
	if issued.IsZero() || issued.Unix() < 0 || issued.Unix() > 1<<62 || prior.State.Sequence == ^uint64(0) {
		return switchwire.Frame{}, errors.New("invalid renewal issue time or exhausted client journal")
	}
	next := prior
	next.Operation, next.Entries = switchwire.OperationCheckin, nil
	next.State.Sequence++
	next.State.Deadline = issued.Unix() + int64(e.IntervalSeconds()) // #nosec G115 -- Enrollment bounds the interval.
	if next.State.Deadline < prior.State.Deadline {
		return switchwire.Frame{}, errors.New("renewal deadline precedes retained authority")
	}
	return next, nil
}

func validRenewalReceipt(receipt switchwire.Envelope, state switchwire.State, e switchwire.AdmittedEnrollment, id string) bool {
	keys := map[string]ed25519.PublicKey{}
	for _, a := range e.Agents {
		keys[a.ID] = a.SigningKey[:]
	}
	f, err := switchwire.VerifyEnvelope(receipt, keys)
	return err == nil && f.Signer == id && f.Operation == switchwire.OperationReceipt && f.State == state && f.Phase == switchwire.PhaseArmed && !f.Released && f.Durable == state.Sequence
}

func renewalView(prior Renewal, f switchwire.Frame, request switchwire.Envelope, receipts map[string]switchwire.Envelope, e switchwire.AdmittedEnrollment) Renewal {
	b0 := prior.B0
	boundary := f.State.Deadline + int64(f.State.Grace) // #nosec G115 -- The authenticated frame bounds grace.
	confirmed := switchwire.Confirm(f.State, orderedReceipts(e, receipts), e, switchwire.PhaseArmed)
	if confirmed {
		b0 = boundary
	}
	r := Renewal{Confirmed: confirmed, B0: b0, Bmax: boundary, Deadline: f.State.Deadline, Grace: f.State.Grace, Allowance: renewalAssumptions, Request: request, Receipts: receipts}
	r.Evidence = switches.RenewalEvidence{ConfirmedRequest: prior.Evidence.ConfirmedRequest, ConfirmedReceipts: prior.Evidence.ConfirmedReceipts, Request: request, Receipts: orderedReceipts(e, receipts)}
	if confirmed {
		r.Evidence.ConfirmedRequest, r.Evidence.ConfirmedReceipts = request, orderedReceipts(e, receipts)
	}
	for _, a := range e.Agents {
		evidence := AgentEvidence{ID: a.ID, State: "uncertain"}
		if receipt, ok := receipts[a.ID]; ok {
			evidence.State, evidence.Receipt = "durable", &receipt
		}
		r.Agents = append(r.Agents, evidence)
	}
	return r
}

func requestPath(path string, sequence uint64) string {
	return fmt.Sprintf("%s/%d.request", path, sequence)
}
func receiptPath(path string, sequence uint64, id string) string {
	return fmt.Sprintf("%s/%d.%s.receipt", path, sequence, id)
}
