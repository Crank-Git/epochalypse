package switches

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"

	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
	bolt "go.etcd.io/bbolt"
)

// ArmEvidence contains public proof of an owner-local arm attempt.
type ArmEvidence struct {
	OwnerKey             [32]byte              `json:"owner_key"`
	Certificate          []byte                `json:"certificate"`
	CertificateSignature []byte                `json:"certificate_signature"`
	Request              switchwire.Envelope   `json:"request"`
	Armed                []switchwire.Envelope `json:"armed"`
}

// ObserveArm retains public evidence without creating an owner signature.
func (s *Store) ObserveArm(ctx context.Context, id string, evidence ArmEvidence) (Record, error) {
	if !validID(id) {
		return Record{}, ErrInput
	}
	var result Record
	err := s.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := recordAt(tx, id)
		if err != nil {
			return err
		}
		if ValidateArmEvidence(record, evidence) != nil || !extendsEvidence(record.Arm, evidence) {
			return ErrInput
		}
		record.Arm = &evidence
		if record.Renewal == nil {
			record.State = evidenceState(evidence)
		}
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if len(data) > MaxRecordBytes {
			return ErrInput
		}
		if err := tx.Bucket([]byte(installation.SwitchesBucket)).Put([]byte(id), data); err != nil {
			return err
		}
		// Charge the replacement once. A quota error rolls back the complete transaction.
		if err := s.db.Admit(tx, 0, 0); err != nil {
			return err
		}
		result, err = decode(id, data)
		if err != nil {
			return err
		}
		return ctx.Err()
	})
	if err != nil {
		return Record{}, err
	}
	return result, nil
}

// ValidateArmEvidence authenticates an initial arm attempt against the retained configuration.
func ValidateArmEvidence(record Record, evidence ArmEvidence) error {
	if !validConfiguration(record) || record.CreatedAt.Unix() < 0 || record.CreatedAt.Unix() >= 1<<62 {
		return ErrInput
	}
	e, err := switchwire.AdmitCertificate(evidence.Certificate, evidence.CertificateSignature, evidence.OwnerKey[:])
	if err != nil || e.IntervalSeconds() != uint64(record.IntervalSeconds) { // #nosec G115 -- The caller validates the configured durations.
		return ErrInput
	}
	f, err := switchwire.VerifyOwnerRequest(evidence.Request, e)
	if err != nil || f.Operation != switchwire.OperationArm || f.State.Switch != record.ID || f.State.Sequence != 1 ||
		f.State.Grace != uint64(record.GraceSeconds) || f.State.Deadline < record.CreatedAt.Unix()+record.IntervalSeconds { // #nosec G115 -- The caller validates the configured durations.
		return ErrInput
	}
	return validateArmedReceipts(evidence.Armed, f.State, e)
}

func validateArmedReceipts(receipts []switchwire.Envelope, state switchwire.State, e switchwire.AdmittedEnrollment) error {
	if len(receipts) > 3 {
		return ErrInput
	}
	keys := map[string]ed25519.PublicKey{}
	for _, agent := range e.Agents {
		keys[agent.ID] = agent.SigningKey[:]
	}
	seen := map[string]bool{}
	for _, receipt := range receipts {
		f, err := switchwire.VerifyEnvelope(receipt, keys)
		if err != nil || seen[f.Signer] || f.Operation != switchwire.OperationReceipt || f.State != state ||
			f.Phase != switchwire.PhaseArmed || f.Released || f.Durable != state.Sequence {
			return ErrInput
		}
		seen[f.Signer] = true
	}
	return nil
}

func evidenceState(evidence ArmEvidence) string {
	if len(evidence.Armed) == 3 {
		return StateArmed
	}
	return StateUncertain
}

func extendsEvidence(previous *ArmEvidence, next ArmEvidence) bool {
	if previous == nil {
		return true
	}
	if previous.OwnerKey != next.OwnerKey || !bytes.Equal(previous.Certificate, next.Certificate) ||
		!bytes.Equal(previous.CertificateSignature, next.CertificateSignature) || !sameEnvelope(previous.Request, next.Request) {
		return false
	}
	for _, receipt := range previous.Armed {
		if !containsEnvelope(next.Armed, receipt) {
			return false
		}
	}
	return true
}

func containsEnvelope(receipts []switchwire.Envelope, wanted switchwire.Envelope) bool {
	for _, receipt := range receipts {
		if sameEnvelope(receipt, wanted) {
			return true
		}
	}
	return false
}

func sameEnvelope(a, b switchwire.Envelope) bool {
	return bytes.Equal(a.Body, b.Body) && bytes.Equal(a.Signature, b.Signature)
}
