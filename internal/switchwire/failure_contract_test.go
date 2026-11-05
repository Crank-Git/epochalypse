package switchwire

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
)

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("synthetic read failure") }

func TestReadEnvelopeReportsReaderFailure(t *testing.T) {
	_, err := ReadEnvelope(failedReader{})
	if err == nil || !strings.Contains(err.Error(), "read switch envelope") || !strings.Contains(err.Error(), "synthetic read failure") {
		t.Fatalf("reader failure lacks context: %v", err)
	}
}

func TestMarshalEnvelopeRejectsInvalidPublicInputs(t *testing.T) {
	frame, key := fixture(t)
	envelope, err := SignFrame(frame, key)
	if err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]Envelope{
		"empty body":      {Signature: envelope.Signature},
		"oversized body":  {Body: bytes.Repeat([]byte{1}, MaxBody+1), Signature: envelope.Signature},
		"short signature": {Body: envelope.Body, Signature: envelope.Signature[:len(envelope.Signature)-1]},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := MarshalEnvelope(candidate); err == nil {
				t.Fatal("marshals invalid envelope")
			}
		})
	}
}

func TestEncodeFrameRejectsSignerRoleSubstitution(t *testing.T) {
	frame, key := fixture(t)
	frame.Operation = OperationReceipt
	frame.Durable = frame.State.Sequence
	if _, err := SignFrame(frame, key); err == nil {
		t.Fatal("owner signs an agent receipt")
	}
	frame.Operation = OperationCheckin
	frame.Signer = "agent1"
	if _, err := EncodeFrame(frame); err == nil {
		t.Fatal("agent signs an owner operation")
	}
}

func TestDecodeFrameRejectsTrailingCanonicalBytes(t *testing.T) {
	frame, _ := fixture(t)
	body, err := EncodeFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeFrame(append(bytes.Clone(body), 0)); err == nil {
		t.Fatal("accepts trailing frame bytes")
	}
}

func TestVerifyEnvelopeRejectsInvalidTrustInputs(t *testing.T) {
	frame, key := fixture(t)
	envelope, err := SignFrame(frame, key)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := key.Public().(ed25519.PublicKey)
	for name, candidate := range map[string]struct {
		envelope Envelope
		keys     map[string]ed25519.PublicKey
	}{
		"missing key":       {envelope: envelope, keys: map[string]ed25519.PublicKey{}},
		"short key":         {envelope: envelope, keys: map[string]ed25519.PublicKey{"owner": publicKey[:len(publicKey)-1]}},
		"short signature":   {envelope: Envelope{Body: envelope.Body, Signature: envelope.Signature[:len(envelope.Signature)-1]}, keys: map[string]ed25519.PublicKey{"owner": publicKey}},
		"wrong signing key": {envelope: envelope, keys: map[string]ed25519.PublicKey{"owner": ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, ed25519.SeedSize)).Public().(ed25519.PublicKey)}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := VerifyEnvelope(candidate.envelope, candidate.keys); err == nil {
				t.Fatal("accepts an untrusted signature")
			}
		})
	}
}

func TestEncodeFrameRejectsInvalidReleaseEvidence(t *testing.T) {
	frame, _ := fixture(t)
	frame.Operation = OperationRelease
	frame.Signer = "agent-a"
	frame.Durable = frame.State.Sequence
	frame.Released = true
	frame.Entries = []Entry{{ID: frame.Signer, Data: bytes.Repeat([]byte{1}, 33)}}
	if _, err := EncodeFrame(frame); err != nil {
		t.Fatalf("valid release evidence: %v", err)
	}
	for name, change := range map[string]func(*Frame){
		"missing released flag":   func(f *Frame) { f.Released = false },
		"wrong durable sequence":  func(f *Frame) { f.Durable++ },
		"wrong evidence identity": func(f *Frame) { f.Entries[0].ID = "agent-b" },
		"short evidence":          func(f *Frame) { f.Entries[0].Data = f.Entries[0].Data[:32] },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := frame
			candidate.Entries = []Entry{{ID: frame.Entries[0].ID, Data: bytes.Clone(frame.Entries[0].Data)}}
			change(&candidate)
			if _, err := EncodeFrame(candidate); err == nil {
				t.Fatal("accepts invalid release evidence")
			}
		})
	}
}
