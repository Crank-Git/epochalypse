package switchwire

import (
	"crypto/ed25519"
	"testing"
)

func TestConfirmRejectsPossibleDisclosure(t *testing.T) {
	cert, owner, seeds := certificateFixture(t)
	body, err := EncodeCertificate(cert)
	if err != nil {
		t.Fatal(err)
	}
	enrolled, err := AdmitCertificate(body, ed25519.Sign(owner, body), owner.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	state := State{Switch: "synthetic", Version: Version, Sequence: 1, Deadline: 100, Grace: cert.Grace, Content: cert.Content, Key: cert.Key, Enrollment: enrolled.Digest}
	receipts := make([]Envelope, 3)
	for i, a := range enrolled.Agents {
		receipts[i], err = SignFrame(Frame{Operation: OperationReceipt, Signer: a.ID, State: state, Durable: 1, Phase: PhaseArmed, Released: true}, ed25519.NewKeyFromSeed(seeds[i][:]))
		if err != nil {
			t.Fatal(err)
		}
	}
	if Confirm(state, receipts, enrolled, PhaseArmed) {
		t.Fatal("confirms arm after possible disclosure")
	}
}
