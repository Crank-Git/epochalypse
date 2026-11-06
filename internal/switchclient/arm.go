// Package switchclient coordinates owner-local switch operations.
package switchclient

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"

	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

// Replica transports authenticated requests to one enrolled agent.
type Replica interface {
	Prepare(context.Context, switchwire.Agent, switchwire.Envelope, []byte, []byte) (switchwire.Envelope, error)
	Arm(context.Context, switchwire.Agent, switchwire.Envelope) (switchwire.Envelope, error)
}

// Plan retains one immutable initial owner intent.
type Plan struct {
	Certificate, CertificateSignature []byte
	Prepare                           switchwire.Envelope
	Ciphertext                        []byte
}

// ArmResult records each response without treating a missing response as rejection.
type ArmResult struct {
	Confirmed       bool
	Prepared, Armed map[string]switchwire.Envelope
	Errors          map[string]string
}

// PrepareAndArm exports verified recovery data before it transmits arm requests.
func PrepareAndArm(ctx context.Context, p Plan, key ed25519.PrivateKey, replicas Replica, bundlePath string) (ArmResult, error) {
	return PrepareAndArmWithIntent(ctx, p, key, replicas, bundlePath, nil)
}

// PrepareAndArmWithIntent registers the durable signed arm intent before the first arm send.
// A registration error prevents transmission. The local bundle and intent remain available for retry.
func PrepareAndArmWithIntent(ctx context.Context, p Plan, key ed25519.PrivateKey, replicas Replica, bundlePath string, beforeArm func(switchwire.Envelope) error) (ArmResult, error) {
	r := ArmResult{Prepared: map[string]switchwire.Envelope{}, Armed: map[string]switchwire.Envelope{}, Errors: map[string]string{}}
	enrollment, frame, err := validatePlan(p, key)
	if err != nil || replicas == nil {
		return r, errors.New("invalid switch arm plan")
	}
	intent, err := json.Marshal(p)
	if err != nil {
		return r, err
	}
	if err := persistExact(bundlePath+".intent", intent); err != nil {
		return r, err
	}
	if err := r.prepareOrResume(ctx, p, enrollment, replicas, bundlePath); err != nil {
		return r, err
	}
	prepared := orderedReceipts(enrollment, r.Prepared)
	if !switchwire.Confirm(frame.State, prepared, enrollment, switchwire.PhasePrepared) {
		return r, errors.New("prepare remains uncertain")
	}
	b := Bundle{Version: 1, Plan: publicPlan(p, frame.State), Prepared: prepared}
	if err := ExportBundle(bundlePath, b, enrollment.OwnerKey[:]); err != nil {
		return r, err
	}
	request, err := signArm(frame, enrollment, prepared, key)
	if err != nil {
		return r, err
	}
	armIntent, err := switchwire.MarshalEnvelope(request)
	if err != nil {
		return r, err
	}
	if err := persistExact(bundlePath+".arm", armIntent); err != nil {
		return r, err
	}
	if beforeArm != nil {
		if err := beforeArm(request); err != nil {
			return r, err
		}
	}
	if err := r.collect(ctx, enrollment, func(a switchwire.Agent) (switchwire.Envelope, error) { return replicas.Arm(ctx, a, request) }, true); err != nil {
		return r, err
	}
	if !switchwire.Confirm(frame.State, orderedReceipts(enrollment, r.Armed), enrollment, switchwire.PhaseArmed) {
		return r, errors.New("arm remains uncertain; disclosure and recovery availability can differ")
	}
	r.Confirmed = true
	return r, nil
}

func (r *ArmResult) prepareOrResume(ctx context.Context, p Plan, e switchwire.AdmittedEnrollment, replicas Replica, path string) error {
	prepared, err := resumePrepared(path, p, e.OwnerKey[:])
	if err != nil {
		return err
	}
	if prepared != nil {
		for _, receipt := range prepared {
			frame, err := switchwire.DecodeFrame(receipt.Body)
			if err != nil {
				return err
			}
			r.Prepared[frame.Signer] = receipt
		}
		return nil
	}
	return r.collect(ctx, e, func(a switchwire.Agent) (switchwire.Envelope, error) {
		return replicas.Prepare(ctx, a, p.Prepare, p.Ciphertext, nil)
	}, false)
}

func (r *ArmResult) collect(ctx context.Context, enrollment switchwire.AdmittedEnrollment, call func(switchwire.Agent) (switchwire.Envelope, error), armed bool) error {
	for _, a := range enrollment.Agents {
		if err := ctx.Err(); err != nil {
			return err
		}
		e, err := call(a)
		if err != nil {
			r.Errors[a.ID] = "agent response unavailable"
			continue
		}
		if armed {
			r.Armed[a.ID] = e
		} else {
			r.Prepared[a.ID] = e
		}
	}
	return nil
}

func signArm(frame switchwire.Frame, enrollment switchwire.AdmittedEnrollment, prepared []switchwire.Envelope, key ed25519.PrivateKey) (switchwire.Envelope, error) {
	frame.Operation, frame.Phase = switchwire.OperationArm, switchwire.PhaseArmed
	frame.Entries = nil
	for i, a := range enrollment.Agents {
		data, err := switchwire.MarshalEnvelope(prepared[i])
		if err != nil {
			return switchwire.Envelope{}, err
		}
		frame.Entries = append(frame.Entries, switchwire.Entry{ID: a.ID, Key: a.SigningKey, Data: data})
	}
	return switchwire.SignFrame(frame, key)
}

func validatePlan(p Plan, key ed25519.PrivateKey) (switchwire.AdmittedEnrollment, switchwire.Frame, error) {
	if len(key) != ed25519.PrivateKeySize {
		return switchwire.AdmittedEnrollment{}, switchwire.Frame{}, errors.New("invalid owner key")
	}
	e, err := switchwire.AdmitCertificate(p.Certificate, p.CertificateSignature, key.Public().(ed25519.PublicKey))
	if err != nil {
		return e, switchwire.Frame{}, err
	}
	f, err := switchwire.VerifyOwnerRequest(p.Prepare, e)
	if err != nil {
		return e, f, err
	}
	if f.Operation != switchwire.OperationPrepare || f.State.Sequence != 1 {
		return e, f, errors.New("invalid prepared content")
	}
	if err := switchcontent.ValidateBinding(p.Ciphertext, f.State.Key, f.State.Content); err != nil {
		return e, f, err
	}
	return e, f, nil
}

func orderedReceipts(e switchwire.AdmittedEnrollment, receipts map[string]switchwire.Envelope) []switchwire.Envelope {
	out := make([]switchwire.Envelope, 0, 3)
	for _, a := range e.Agents {
		if receipt, ok := receipts[a.ID]; ok {
			out = append(out, receipt)
		}
	}
	return out
}
