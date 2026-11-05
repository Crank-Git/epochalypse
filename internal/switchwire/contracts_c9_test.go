package switchwire

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestSignFrameRejectsInvalidPrivateKeyLength(t *testing.T) {
	frame, key := fixture(t)
	for _, candidate := range []ed25519.PrivateKey{
		nil,
		key[:len(key)-1],
		append(bytes.Clone(key), 0),
	} {
		if _, err := SignFrame(frame, candidate); err == nil {
			t.Fatal("signs with a private key of invalid length")
		}
	}
}

func TestAdmitCertificateRejectsInvalidPolicyValues(t *testing.T) {
	for name, change := range map[string]func(*Certificate){
		"empty owner key":      func(c *Certificate) { c.OwnerKey = [32]byte{} },
		"empty content key":    func(c *Certificate) { c.Key = [32]byte{} },
		"empty content digest": func(c *Certificate) { c.Content = [32]byte{} },
		"unsupported version":  func(c *Certificate) { c.Version++ },
		"short interval":       func(c *Certificate) { c.Interval = 3599 },
		"long interval":        func(c *Certificate) { c.Interval = 31536001 },
		"long grace period":    func(c *Certificate) { c.Grace = 2592001 },
	} {
		t.Run(name, func(t *testing.T) {
			certificate, _, _ := certificateFixture(t)
			change(&certificate)
			if _, err := EncodeCertificate(certificate); err == nil {
				t.Fatal("encodes a certificate with invalid policy values")
			}
		})
	}
}

func TestVerifyOwnerRequestRejectsUnadmittedEnrollment(t *testing.T) {
	frame, key := fixture(t)
	envelope, err := SignFrame(frame, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyOwnerRequest(envelope, AdmittedEnrollment{}); err == nil {
		t.Fatal("accepts an enrollment without owner verification")
	}
}

func TestVerifyOwnerRequestRejectsInvalidArmReceiptEvidence(t *testing.T) {
	certificate, ownerKey, agentSeeds := certificateFixture(t)
	certificateBody, err := EncodeCertificate(certificate)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := AdmitCertificate(certificateBody, ed25519.Sign(ownerKey, certificateBody), ownerKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	state := State{Switch: "synthetic", Version: Version, Sequence: 1, Deadline: 100, Grace: certificate.Grace, Content: certificate.Content, Key: certificate.Key, Enrollment: enrollment.Digest}

	tests := map[string]func(*testing.T, *Frame){
		"unknown agent identity": func(t *testing.T, frame *Frame) {
			frame.Entries[0].ID = "agent-0"
			frame.Entries[0].Key[0]++
		},
		"malformed receipt envelope": func(t *testing.T, frame *Frame) {
			frame.Entries[0].Data = []byte{1}
		},
		"receipt has wrong phase": func(t *testing.T, frame *Frame) {
			frame.Entries[0].Data = mustArmReceipt(t, state, enrollment.Agents[0], agentSeeds[0], PhaseArmed)
		},
		"receipt has wrong signer": func(t *testing.T, frame *Frame) {
			frame.Entries[0].Data = mustArmReceipt(t, state, enrollment.Agents[1], agentSeeds[1], PhasePrepared)
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			arm := Frame{Operation: OperationArm, Signer: "owner", State: state, Phase: PhaseArmed, Entries: make([]Entry, 3)}
			for i, agent := range enrollment.Agents {
				arm.Entries[i] = Entry{ID: agent.ID, Key: agent.SigningKey, Data: mustArmReceipt(t, state, agent, agentSeeds[i], PhasePrepared)}
			}
			change(t, &arm)
			envelope, err := SignFrame(arm, ownerKey)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyOwnerRequest(envelope, enrollment); err == nil {
				t.Fatal("accepts invalid arm receipt evidence")
			}
		})
	}
}

func mustArmReceipt(t *testing.T, state State, agent Agent, seed [32]byte, phase byte) []byte {
	t.Helper()
	envelope, err := SignFrame(Frame{Operation: OperationReceipt, Signer: agent.ID, State: state, Durable: state.Sequence, Phase: phase}, ed25519.NewKeyFromSeed(seed[:]))
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
