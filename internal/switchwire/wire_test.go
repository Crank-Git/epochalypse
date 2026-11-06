package switchwire

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"math"
	"testing"

	"filippo.io/age"
)

func fixture(t *testing.T) (Frame, ed25519.PrivateKey) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32))
	return Frame{Operation: OperationCheckin, Signer: "owner", State: State{Switch: "synthetic", Version: Version, Sequence: 1, Deadline: 100, Grace: 10, Content: [32]byte{1}, Key: [32]byte{2}, Enrollment: [32]byte{3}}}, key
}

func TestEncodeFramePreservesReviewedVector(t *testing.T) {
	f, _ := fixture(t)
	b, err := EncodeFrame(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 231 || sha256.Sum256(b) != [32]byte{0xa2, 0x58, 0xf5, 0xcc, 0xfb, 0x10, 0x67, 0x3b, 0xc7, 0xa3, 0x0c, 0x48, 0xe6, 0xb7, 0x64, 0x31, 0x16, 0xa1, 0x17, 0x3d, 0xe1, 0x1c, 0x93, 0x58, 0xf5, 0xec, 0x66, 0xca, 0xda, 0xdd, 0x14, 0x97} {
		t.Fatal("version two bytes differ")
	}
}

func TestReadEnvelopeUsesBoundedLengthFrame(t *testing.T) {
	f, key := fixture(t)
	env, err := SignFrame(f, key)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadEnvelope(bytes.NewReader(wire))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ReadEnvelope(bytes.NewReader(append(wire, 0))); err == nil {
		t.Fatal("accepts excess stream bytes")
	}
	if _, err = ReadEnvelope(bytes.NewReader(wire[:len(wire)-1])); err == nil {
		t.Fatal("accepts truncated envelope")
	}
	if _, err = ReadEnvelope(io.LimitReader(bytes.NewReader(wire), 2)); err == nil {
		t.Fatal("accepts truncated length")
	}
	for _, bad := range [][]byte{{0, 0, 0, 0}, {0xff, 0xff, 0xff, 0xff}, {0, 0, 0, 1}} {
		if _, err = ReadEnvelope(bytes.NewReader(bad)); err == nil {
			t.Fatal("accepts malformed envelope framing")
		}
	}
	if got.Body == nil {
		t.Fatal("loses frame body")
	}
}

func TestVerifyEnvelopeRejectsChangedAndUnenrolledFrames(t *testing.T) {
	f, key := fixture(t)
	env, err := SignFrame(f, key)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{"owner": key.Public().(ed25519.PublicKey)}
	if _, err = VerifyEnvelope(env, keys); err != nil {
		t.Fatal(err)
	}
	env.Body = bytes.Clone(env.Body)
	env.Body[len(env.Body)-1] ^= 1
	if _, err = VerifyEnvelope(env, keys); err == nil {
		t.Fatal("accepts changed frame")
	}
	delete(keys, "owner")
	if _, err = VerifyEnvelope(Envelope{Body: mustFrame(t, f), Signature: ed25519.Sign(key, mustFrame(t, f))}, keys); err == nil {
		t.Fatal("accepts unknown signer")
	}
}

func TestEncodeFrameRejectsInvalidStateAndOperationFields(t *testing.T) {
	base, _ := fixture(t)
	changes := []func(*Frame){
		func(f *Frame) { f.State.Version++ },
		func(f *Frame) { f.State.Sequence = 0 },
		func(f *Frame) { f.State.Deadline = -1 },
		func(f *Frame) { f.State.Grace = math.MaxUint64 },
		func(f *Frame) { f.Phase = PhaseStopped + 1 },
		func(f *Frame) { f.Operation = "unknown" },
		func(f *Frame) { f.Signer = "agent-a" },
		func(f *Frame) { f.Operation = OperationReceipt; f.Signer = "owner"; f.Durable = 1 },
		func(f *Frame) { f.Operation = OperationReceipt; f.Signer = "agent-a"; f.Durable = 2 },
		func(f *Frame) { f.Operation = OperationPrepare; f.Entries = []Entry{{ID: "agent-a", Data: []byte{1}}} },
		func(f *Frame) {
			f.Entries = []Entry{{ID: "agent-a", Data: []byte{1}}, {ID: "agent-a", Data: []byte{1}}}
		},
	}
	for i, change := range changes {
		f := base
		change(&f)
		if _, err := EncodeFrame(f); err == nil {
			t.Errorf("accepts invalid frame case %d", i)
		}
	}
	body, err := EncodeFrame(base)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(body); i++ {
		if _, err = DecodeFrame(body[:i]); err == nil {
			t.Fatalf("accepts truncated frame at %d", i)
		}
	}
}

func TestAdmitCertificateVerifiesCanonicalOwnerSignature(t *testing.T) {
	cert, ownerKey, agentKeys := certificateFixture(t)
	body, err := EncodeCertificate(cert)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(ownerKey, body)
	admitted, err := AdmitCertificate(body, sig, ownerKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	if admitted.Digest != sha256.Sum256(body) || len(admitted.Agents) != 3 {
		t.Fatal("admission differs")
	}
	for _, mutate := range []func([]byte) []byte{
		func(b []byte) []byte { return append(bytes.Clone(b), 0) },
		func(b []byte) []byte { n := bytes.Clone(b); n[0] ^= 1; return n },
	} {
		if _, err := AdmitCertificate(mutate(body), sig, ownerKey.Public().(ed25519.PublicKey)); err == nil {
			t.Fatal("admits changed certificate")
		}
	}
	wrong := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, 32))
	if _, err := AdmitCertificate(body, sig, wrong.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("admits wrong owner key")
	}
	if agentKeys[0] == [32]byte{} {
		t.Fatal("fixture lacks agent key")
	}
}

func TestAdmitCertificateRejectsRepeatedAgentKeysAndRecipients(t *testing.T) {
	for _, duplicate := range []string{"identity", "signature key", "recipient"} {
		t.Run(duplicate, func(t *testing.T) {
			cert, ownerKey, _ := certificateFixture(t)
			switch duplicate {
			case "identity":
				cert.Agents[1].ID = cert.Agents[0].ID
			case "signature key":
				cert.Agents[1].SigningKey = cert.Agents[0].SigningKey
			case "recipient":
				cert.Agents[1].Recipient = cert.Agents[0].Recipient
			}
			body, err := EncodeCertificate(cert)
			if err != nil {
				return
			}
			if _, err = AdmitCertificate(body, ed25519.Sign(ownerKey, body), ownerKey.Public().(ed25519.PublicKey)); err == nil {
				t.Fatal("admits repeated agent material")
			}
		})
	}
}

func TestAdmitCertificateRejectsUnsafeRecoveryEndpoints(t *testing.T) {
	for _, endpoint := range []string{"http://agent.example.test/recovery", "https://user:pass@agent.example.test/recovery", "https://agent.example.test/recovery#fragment", "https://agent.example.test/with space"} {
		t.Run(endpoint, func(t *testing.T) {
			cert, _, _ := certificateFixture(t)
			cert.Agents[0].Endpoint = endpoint
			if _, err := EncodeCertificate(cert); err == nil {
				t.Fatal("encodes unsafe recovery endpoint")
			}
		})
	}
}

func TestConfirmRequiresThreeDistinctSignedReceipts(t *testing.T) {
	cert, ownerKey, seeds := certificateFixture(t)
	body, err := EncodeCertificate(cert)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := AdmitCertificate(body, ed25519.Sign(ownerKey, body), ownerKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	state := State{Switch: "synthetic", Version: Version, Sequence: 1, Deadline: 100, Grace: 10, Content: cert.Content, Key: cert.Key, Enrollment: enrollment.Digest}
	receipts := make([]Envelope, 3)
	for i, agent := range enrollment.Agents {
		key := ed25519.NewKeyFromSeed(seeds[i][:])
		receipts[i], err = SignFrame(Frame{Operation: OperationReceipt, Signer: agent.ID, State: state, Durable: state.Sequence, Phase: PhaseArmed}, key)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !Confirm(state, receipts, enrollment, PhaseArmed) {
		t.Fatal("rejects three matching receipts")
	}
	if Confirm(state, receipts[:2], enrollment, PhaseArmed) {
		t.Fatal("confirms fewer than three receipts")
	}
	if Confirm(state, []Envelope{receipts[0], receipts[0], receipts[2]}, enrollment, PhaseArmed) {
		t.Fatal("confirms duplicate signer")
	}
	wrong := state
	wrong.Sequence++
	if Confirm(wrong, receipts, enrollment, PhaseArmed) {
		t.Fatal("confirms receipts for another state")
	}
	if Confirm(state, receipts, enrollment, PhasePrepared) {
		t.Fatal("confirms another lifecycle phase")
	}
	wrong = state
	wrong.Key[0]++
	if Confirm(wrong, receipts, enrollment, PhaseArmed) {
		t.Fatal("confirms receipts for another enrolled key")
	}
	wrong = state
	wrong.Content[0]++
	if Confirm(wrong, receipts, enrollment, PhaseArmed) {
		t.Fatal("confirms receipts for another content digest")
	}
	wrong = state
	wrong.Grace++
	if Confirm(wrong, receipts, enrollment, PhaseArmed) {
		t.Fatal("confirms receipts for another grace period")
	}
	changed := enrollment
	changed.Agents[0].ID = "other-agent"
	if Confirm(state, receipts, changed, PhaseArmed) {
		t.Fatal("accepts changed admitted enrollment")
	}
}

func TestVerifyOwnerRequestBindsPreparedEntriesAndArmReceipts(t *testing.T) {
	cert, ownerKey, agentKeys := certificateFixture(t)
	body, err := EncodeCertificate(cert)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := AdmitCertificate(body, ed25519.Sign(ownerKey, body), ownerKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	state := State{Switch: "synthetic", Version: Version, Sequence: 1, Deadline: 100, Grace: 10, Content: cert.Content, Key: cert.Key, Enrollment: enrollment.Digest}
	testPrepareOwnerRequest(t, state, ownerKey, enrollment)
	testArmOwnerRequest(t, state, ownerKey, enrollment, agentKeys)
}

func testPrepareOwnerRequest(t *testing.T, state State, ownerKey ed25519.PrivateKey, enrollment AdmittedEnrollment) {
	t.Helper()
	prepared := Frame{Operation: OperationPrepare, Signer: "owner", State: state, Phase: PhasePrepared, Entries: make([]Entry, 3)}
	for i, agent := range enrollment.Agents {
		recipient, err := age.ParseX25519Recipient(agent.Recipient)
		if err != nil {
			t.Fatal(err)
		}
		prepared.Entries[i] = Entry{ID: agent.ID, Key: sha256.Sum256([]byte(recipient.String())), Data: []byte{1}}
	}
	env, err := SignFrame(prepared, ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyOwnerRequest(env, enrollment); err != nil {
		t.Fatal(err)
	}
	prepared.Entries[0].Key[0]++
	env, err = SignFrame(prepared, ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyOwnerRequest(env, enrollment); err == nil {
		t.Fatal("accepts recipient fingerprint mismatch")
	}
	for _, mutate := range []func(*Frame){
		func(f *Frame) { f.Phase = PhaseDraft },
		func(f *Frame) { f.Released = true },
		func(f *Frame) { f.State.Key[0]++ },
		func(f *Frame) { f.State.Content[0]++ },
		func(f *Frame) { f.State.Grace++ },
	} {
		candidate := prepared
		for i := range candidate.Entries {
			candidate.Entries[i].Key = sha256.Sum256([]byte(candidateEntryRecipient(t, enrollment.Agents[i].Recipient)))
		}
		mutate(&candidate)
		env, err = SignFrame(candidate, ownerKey)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = VerifyOwnerRequest(env, enrollment); err == nil {
			t.Fatal("accepts prepare state outside enrollment")
		}
	}
}

func testArmOwnerRequest(t *testing.T, state State, ownerKey ed25519.PrivateKey, enrollment AdmittedEnrollment, agentKeys [3][32]byte) {
	t.Helper()
	arm := Frame{Operation: OperationArm, Signer: "owner", State: state, Phase: PhaseArmed, Entries: make([]Entry, 3)}
	for i, agent := range enrollment.Agents {
		receipt, err := SignFrame(Frame{Operation: OperationReceipt, Signer: agent.ID, State: state, Durable: state.Sequence, Phase: PhasePrepared}, ed25519.NewKeyFromSeed(agentKeys[i][:]))
		if err != nil {
			t.Fatal(err)
		}
		data, err := MarshalEnvelope(receipt)
		if err != nil {
			t.Fatal(err)
		}
		arm.Entries[i] = Entry{ID: agent.ID, Key: agent.SigningKey, Data: data}
	}
	env, err := SignFrame(arm, ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyOwnerRequest(env, enrollment); err != nil {
		t.Fatal(err)
	}
	arm.Released = true
	env, err = SignFrame(arm, ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyOwnerRequest(env, enrollment); err == nil {
		t.Fatal("accepts disclosive arm request")
	}
	arm.Released = false
	unsafeReceipt, err := SignFrame(Frame{Operation: OperationReceipt, Signer: enrollment.Agents[0].ID, State: state, Durable: state.Sequence, Phase: PhasePrepared, Released: true}, ed25519.NewKeyFromSeed(agentKeys[0][:]))
	if err != nil {
		t.Fatal(err)
	}
	arm.Entries[0].Data, err = MarshalEnvelope(unsafeReceipt)
	if err != nil {
		t.Fatal(err)
	}
	env, err = SignFrame(arm, ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyOwnerRequest(env, enrollment); err == nil {
		t.Fatal("accepts disclosive prepared receipt")
	}
	arm.Entries[1].Data = bytes.Clone(arm.Entries[0].Data)
	env, err = SignFrame(arm, ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyOwnerRequest(env, enrollment); err == nil {
		t.Fatal("accepts duplicate nested receipt")
	}
}

func candidateEntryRecipient(t *testing.T, recipient string) string {
	t.Helper()
	parsed, err := age.ParseX25519Recipient(recipient)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.String()
}

func certificateFixture(t *testing.T) (Certificate, ed25519.PrivateKey, [3][32]byte) {
	t.Helper()
	owner := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32))
	c := Certificate{OwnerKey: [32]byte(owner.Public().(ed25519.PublicKey)), Key: [32]byte{2}, Content: [32]byte{1}, Interval: 3600, Grace: 10, Version: Version}
	var keys [3][32]byte
	for i, id := range [3]string{"agent-a", "agent-b", "agent-c"} {
		key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{byte(i + 10)}, 32))
		keys[i] = [32]byte(key.Seed())
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		c.Agents[i] = Agent{ID: id, SigningKey: [32]byte(key.Public().(ed25519.PublicKey)), Recipient: identity.Recipient().String(), Endpoint: "https://" + id + ".example.test/recovery"}
	}
	return c, owner, keys
}

func mustFrame(t *testing.T, f Frame) []byte {
	t.Helper()
	b, err := EncodeFrame(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMarshalEnvelopeEncodesFourByteLength(t *testing.T) {
	f, key := fixture(t)
	e, err := SignFrame(f, key)
	if err != nil {
		t.Fatal(err)
	}
	b, err := MarshalEnvelope(e)
	if err != nil {
		t.Fatal(err)
	}
	if int64(binary.BigEndian.Uint32(b[:4])) != int64(len(e.Body)) {
		t.Fatal("length prefix differs")
	}
}
