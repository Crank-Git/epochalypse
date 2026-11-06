package switches

import (
	"context"
	"encoding/json"

	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
	bolt "go.etcd.io/bbolt"
)

// RenewalEvidence contains public proof of confirmed authority and issued intent.
type RenewalEvidence struct {
	ConfirmedRequest  switchwire.Envelope   `json:"confirmed_request"`
	ConfirmedReceipts []switchwire.Envelope `json:"confirmed_receipts"`
	Request           switchwire.Envelope   `json:"request"`
	Receipts          []switchwire.Envelope `json:"receipts"`
}

// AgentEvidence distinguishes a durable receipt from an absent response.
type AgentEvidence struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// RenewalStatus contains independently derived disclosure bounds.
type RenewalStatus struct {
	Confirmed bool            `json:"confirmed"`
	B0        int64           `json:"b0"`
	Bmax      int64           `json:"bmax"`
	Deadline  int64           `json:"deadline"`
	Grace     uint64          `json:"grace_seconds"`
	Allowance string          `json:"allowance_assumptions"`
	Agents    []AgentEvidence `json:"agents"`
}

// RenewalStatus derives each bound from authenticated retained evidence.
func (r Record) RenewalStatus() *RenewalStatus {
	if r.Renewal == nil {
		return nil
	}
	e, prior, current, err := validateRenewal(r, *r.Renewal)
	if err != nil {
		return nil
	}
	confirmed := len(r.Renewal.Receipts) == 3
	b0 := prior.State.Deadline + int64(prior.State.Grace)       // #nosec G115 -- Verified frames bound grace.
	bmax := current.State.Deadline + int64(current.State.Grace) // #nosec G115 -- Verified frames bound grace.
	if confirmed {
		b0 = bmax
	}
	status := &RenewalStatus{Confirmed: confirmed, B0: b0, Bmax: bmax, Deadline: current.State.Deadline, Grace: current.State.Grace, Allowance: "E is separate. Candidate E=34 seconds requires reviewed clock and publication premises. Actual hosts have no numeric acceptance. Main loss plus one outage requires two reachable, honest agents with intact durable state."}
	for _, a := range e.Agents {
		state := "uncertain"
		for _, receipt := range r.Renewal.Receipts {
			f, _ := switchwire.DecodeFrame(receipt.Body)
			if f.Signer == a.ID {
				state = "durable"
			}
		}
		status.Agents = append(status.Agents, AgentEvidence{ID: a.ID, State: state})
	}
	return status
}

func validateRenewal(r Record, proof RenewalEvidence) (switchwire.AdmittedEnrollment, switchwire.Frame, switchwire.Frame, error) {
	var empty switchwire.AdmittedEnrollment
	var frame switchwire.Frame
	if r.Arm == nil || ValidateArmEvidence(r, *r.Arm) != nil || len(r.Arm.Armed) != 3 {
		return empty, frame, frame, ErrInput
	}
	e, err := switchwire.AdmitCertificate(r.Arm.Certificate, r.Arm.CertificateSignature, r.Arm.OwnerKey[:])
	if err != nil {
		return e, frame, frame, ErrInput
	}
	arm, _ := switchwire.VerifyOwnerRequest(r.Arm.Request, e)
	prior, err := renewalFrame(proof.ConfirmedRequest, e, arm.State)
	if err != nil || len(proof.ConfirmedReceipts) != 3 || validateArmedReceipts(proof.ConfirmedReceipts, prior.State, e) != nil {
		return e, frame, frame, ErrInput
	}
	current, err := renewalFrame(proof.Request, e, arm.State)
	if err != nil || current.Operation != switchwire.OperationCheckin || current.State.Sequence < prior.State.Sequence || current.State.Deadline < prior.State.Deadline || validateArmedReceipts(proof.Receipts, current.State, e) != nil {
		return e, frame, frame, ErrInput
	}
	if !followsConfirmed(proof, prior.State.Sequence, current.State.Sequence) {
		return e, frame, frame, ErrInput
	}
	return e, prior, current, nil
}

func followsConfirmed(proof RenewalEvidence, prior, current uint64) bool {
	if current == prior {
		return sameEnvelope(proof.Request, proof.ConfirmedRequest) && len(proof.Receipts) == 3
	}
	return prior != ^uint64(0) && current == prior+1
}

func renewalFrame(request switchwire.Envelope, e switchwire.AdmittedEnrollment, arm switchwire.State) (switchwire.Frame, error) {
	f, err := switchwire.VerifyOwnerRequest(request, e)
	if err != nil {
		return f, ErrInput
	}
	base := f.State
	base.Sequence, base.Deadline = arm.Sequence, arm.Deadline
	if base != arm || f.State.Sequence < 1 || f.State.Deadline < arm.Deadline || f.Phase != switchwire.PhaseArmed || f.Released {
		return f, ErrInput
	}
	operation := switchwire.OperationCheckin
	if f.State.Sequence == 1 {
		operation = switchwire.OperationArm
		if f.State != arm {
			return f, ErrInput
		}
	}
	if f.Operation != operation {
		return f, ErrInput
	}
	return f, nil
}

func extendsRenewal(r Record, next RenewalEvidence) bool {
	if r.Renewal == nil {
		return true
	}
	old := r.Renewal
	before, _ := switchwire.DecodeFrame(old.Request.Body)
	after, _ := switchwire.DecodeFrame(next.Request.Body)
	baseline, _ := switchwire.DecodeFrame(next.ConfirmedRequest.Body)
	oldBaseline, _ := switchwire.DecodeFrame(old.ConfirmedRequest.Body)
	if after.State.Sequence < before.State.Sequence || after.State.Deadline < before.State.Deadline || baseline.State.Sequence < oldBaseline.State.Sequence || baseline.State.Deadline < oldBaseline.State.Deadline {
		return false
	}
	if after.State.Sequence == before.State.Sequence {
		if !sameEnvelope(next.Request, old.Request) {
			return false
		}
		for _, receipt := range old.Receipts {
			if !containsEnvelope(next.Receipts, receipt) {
				return false
			}
		}
		return true
	}
	if baseline.State.Sequence < before.State.Sequence || baseline.State.Deadline < before.State.Deadline {
		return false
	}
	return baseline.State.Sequence != before.State.Sequence || sameEnvelope(next.ConfirmedRequest, old.Request)
}

// ObserveRenewal authenticates public evidence without sending an agent request.
func (s *Store) ObserveRenewal(ctx context.Context, id string, proof RenewalEvidence) (Record, error) {
	if !validID(id) {
		return Record{}, ErrInput
	}
	var result Record
	err := s.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, err := recordAt(tx, id)
		if err != nil {
			return err
		}
		if _, _, _, err := validateRenewal(r, proof); err != nil || !extendsRenewal(r, proof) {
			return ErrInput
		}
		r.Renewal = &proof
		r.State = StateUncertain
		if len(proof.Receipts) == 3 {
			r.State = StateArmed
		}
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if len(data) > MaxRecordBytes {
			return ErrInput
		}
		if err := tx.Bucket([]byte(installation.SwitchesBucket)).Put([]byte(id), data); err != nil {
			return err
		}
		if err := s.db.Admit(tx, 0, 0); err != nil {
			return err
		}
		result, err = decode(id, data)
		if err != nil {
			return err
		}
		return ctx.Err()
	})
	return result, err
}
