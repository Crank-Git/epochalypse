package switchwire

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestSignedCertificateRejectsEveryTruncatedPrefix(t *testing.T) {
	certificate, key, _ := certificateFixture(t)
	body, err := EncodeCertificate(certificate)
	if err != nil {
		t.Fatal(err)
	}
	owner := key.Public().(ed25519.PublicKey)
	for end := 0; end < len(body); end++ {
		prefix := body[:end]
		if _, err := AdmitCertificate(prefix, ed25519.Sign(key, prefix), owner); err == nil {
			t.Fatalf("truncated certificate is admitted at byte %d", end)
		}
	}
}

func TestSignedFrameRejectsEveryTruncatedPrefix(t *testing.T) {
	f, key := fixture(t)
	body, err := EncodeFrame(f)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{"owner": key.Public().(ed25519.PublicKey)}
	for end := 0; end < len(body); end++ {
		prefix := body[:end]
		if _, err := VerifyEnvelope(Envelope{Body: prefix, Signature: ed25519.Sign(key, prefix)}, keys); err == nil {
			t.Fatalf("truncated frame is admitted at byte %d", end)
		}
	}
}

func TestOwnerRequestRejectsSubstitutedEnrollmentSnapshot(t *testing.T) {
	c, key, _ := certificateFixture(t)
	body, err := EncodeCertificate(c)
	if err != nil {
		t.Fatal(err)
	}
	e, err := AdmitCertificate(body, ed25519.Sign(key, body), key.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	f, _ := fixture(t)
	f.State.Enrollment, f.State.Key, f.State.Content, f.State.Grace = e.Digest, c.Key, c.Content, c.Grace
	request, err := SignFrame(f, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyOwnerRequest(request, e); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"digest", "owner", "agent", "unadmitted", "agent-role"} {
		t.Run(kind, func(t *testing.T) {
			changed, candidate := e, request
			switch kind {
			case "digest":
				changed.Digest[0] ^= 1
			case "owner":
				changed.OwnerKey[0] ^= 1
			case "agent":
				changed.Agents[0].Endpoint = "https://substituted.invalid"
			case "unadmitted":
				changed = AdmittedEnrollment{}
			case "agent-role":
				frame := f
				frame.Operation, frame.Signer, frame.Durable = OperationReceipt, c.Agents[0].ID, frame.State.Sequence
				candidate, err = SignFrame(frame, key)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := VerifyOwnerRequest(candidate, changed); err == nil {
				t.Fatal("substituted authority is admitted")
			}
			if Confirm(f.State, []Envelope{candidate}, changed, PhasePrepared) {
				t.Fatal("substituted authority confirms state")
			}
		})
	}
	if _, err := ReadEnvelope(bytes.NewReader(make([]byte, MaxEnvelope+1))); err == nil {
		t.Fatal("oversized envelope is admitted")
	}
}
