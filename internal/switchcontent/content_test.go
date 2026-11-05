package switchcontent

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"testing"

	"filippo.io/age"
)

const testContentLimit = 10 << 20

func TestPrepareRecoversContentFromAnyTwoShares(t *testing.T) {
	plaintext := []byte("switch content")
	recipients, identities := testRecipients(t)

	material, err := Prepare(bytes.NewReader(plaintext), recipients)
	if err != nil {
		t.Fatalf("prepare content: %v", err)
	}
	if material.ContentDigest != sha256.Sum256(material.Ciphertext) {
		t.Fatal("content digest does not match ciphertext")
	}
	if material.KeyID == ([32]byte{}) {
		t.Fatal("key identity is empty")
	}

	var shares [3][]byte
	for i := range identities {
		shares[i], err = DecryptShare(material.Shares[i], identities[i])
		if err != nil {
			t.Fatalf("decrypt share %d: %v", i, err)
		}
		for j, identity := range identities {
			if i != j {
				if _, err := DecryptShare(material.Shares[i], identity); err == nil {
					t.Fatalf("identity %d decrypted share %d", j, i)
				}
			}
		}
	}
	for _, pair := range [][2]int{{0, 1}, {0, 2}, {1, 2}} {
		recovered, err := Recover(material.Ciphertext, [][]byte{shares[pair[0]], shares[pair[1]]})
		if err != nil {
			t.Fatalf("recover from shares %v: %v", pair, err)
		}
		if !bytes.Equal(recovered, plaintext) {
			t.Fatalf("shares %v recovered different content", pair)
		}
	}
}

func TestPrepareAcceptsContentAtLimit(t *testing.T) {
	plaintext := bytes.Repeat([]byte("x"), testContentLimit)
	recipients, _ := testRecipients(t)

	if _, err := Prepare(bytes.NewReader(plaintext), recipients); err != nil {
		t.Fatalf("prepare content at limit: %v", err)
	}
}

func TestPrepareRejectsContentOverLimit(t *testing.T) {
	tooLarge := bytes.NewReader(make([]byte, testContentLimit+1))
	recipients, _ := testRecipients(t)

	if _, err := Prepare(tooLarge, recipients); err == nil {
		t.Fatal("content over the limit was accepted")
	}
}

func TestPrepareRejectsDuplicateRecipients(t *testing.T) {
	recipients, _ := testRecipients(t)
	recipients[2] = recipients[0]

	if _, err := Prepare(bytes.NewReader([]byte("content")), recipients); err == nil {
		t.Fatal("duplicate recipient was accepted")
	}
}

func TestPrepareRejectsNonCanonicalRecipient(t *testing.T) {
	recipients, _ := testRecipients(t)
	recipients[0] = strings.ToUpper(recipients[0])

	if _, err := Prepare(bytes.NewReader([]byte("content")), recipients); err == nil {
		t.Fatal("non-canonical recipient was accepted")
	}
}

func TestDecryptShareRejectsWrongIdentity(t *testing.T) {
	recipients, identities := testRecipients(t)
	material, err := Prepare(bytes.NewReader([]byte("content")), recipients)
	if err != nil {
		t.Fatalf("prepare content: %v", err)
	}

	if _, err := DecryptShare(material.Shares[0], identities[1]); err == nil {
		t.Fatal("a different identity decrypted the share")
	}
}

func TestDecryptShareRejectsInvalidEnvelopeSize(t *testing.T) {
	_, identities := testRecipients(t)
	for _, envelope := range [][]byte{nil, make([]byte, maxShareEnvelopeBytes+1)} {
		if _, err := DecryptShare(envelope, identities[0]); err == nil {
			t.Fatalf("envelope of size %d was accepted", len(envelope))
		}
	}
}

func TestRecoverRejectsChangedCiphertext(t *testing.T) {
	material, shares := preparedContent(t, []byte("content"))
	material.Ciphertext[len(material.Ciphertext)-1] ^= 0x01

	if _, err := Recover(material.Ciphertext, shares[:2]); err == nil {
		t.Fatal("changed ciphertext was accepted")
	}
}

func TestRecoverRejectsCorruptShare(t *testing.T) {
	material, shares := preparedContent(t, []byte("content"))
	shares[0][1] ^= 0x01

	if _, err := Recover(material.Ciphertext, shares[:2]); err == nil {
		t.Fatal("corrupt share was accepted")
	}
}

func TestRecoverRejectsDuplicateShareCoordinates(t *testing.T) {
	material, shares := preparedContent(t, []byte("content"))
	shares[1][len(shares[1])-1] = shares[0][len(shares[0])-1]

	_, err := Recover(material.Ciphertext, shares[:2])
	if err == nil || err.Error() != "recovery shares are not distinct" {
		t.Fatalf("recover error = %v, want duplicate-coordinate error", err)
	}
}

func TestRecoverDoesNotTreatShareDataAsCoordinate(t *testing.T) {
	material, shares := preparedContent(t, []byte("content"))
	shares[1][0] = shares[0][0]

	_, err := Recover(material.Ciphertext, shares[:2])
	if err == nil || err.Error() == "recovery shares are not distinct" {
		t.Fatalf("recover error = %v, want authenticated recovery failure", err)
	}
}

func TestValidateBindingAcceptsPreparedMaterial(t *testing.T) {
	material, _ := preparedContent(t, []byte("content"))

	if err := ValidateBinding(material.Ciphertext, material.KeyID, material.ContentDigest); err != nil {
		t.Fatalf("validate prepared material: %v", err)
	}
}

func TestValidateBindingRejectsWrongKeyIdentity(t *testing.T) {
	material, _ := preparedContent(t, []byte("content"))
	keyID := material.KeyID
	keyID[0] ^= 1

	if err := ValidateBinding(material.Ciphertext, keyID, material.ContentDigest); err == nil {
		t.Fatal("wrong key identity was accepted")
	}
}

func TestValidateBindingRejectsWrongCiphertextDigest(t *testing.T) {
	material, _ := preparedContent(t, []byte("content"))
	digest := material.ContentDigest
	digest[0] ^= 1

	if err := ValidateBinding(material.Ciphertext, material.KeyID, digest); err == nil {
		t.Fatal("wrong ciphertext digest was accepted")
	}
}

func TestRecoverRejectsShareFromDifferentContent(t *testing.T) {
	material, shares := preparedContent(t, []byte("content"))
	_, otherShares := preparedContent(t, []byte("other content"))

	if _, err := Recover(material.Ciphertext, [][]byte{shares[0], otherShares[0]}); err == nil {
		t.Fatal("shares from different content were accepted")
	}
}

func TestPrepareReturnsReaderFailure(t *testing.T) {
	recipients, _ := testRecipients(t)
	reader := failingReader{}

	_, err := Prepare(reader, recipients)
	if !errors.Is(err, errTestReader) {
		t.Fatalf("prepare error = %v, want reader cause", err)
	}
}

var errTestReader = errors.New("test reader failure")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errTestReader }

var _ io.Reader = failingReader{}

func preparedContent(t *testing.T, plaintext []byte) (Material, [][]byte) {
	t.Helper()
	recipients, identities := testRecipients(t)
	material, err := Prepare(bytes.NewReader(plaintext), recipients)
	if err != nil {
		t.Fatalf("prepare content: %v", err)
	}
	shares := make([][]byte, len(identities))
	for i := range identities {
		shares[i], err = DecryptShare(material.Shares[i], identities[i])
		if err != nil {
			t.Fatalf("decrypt share %d: %v", i, err)
		}
	}
	return material, shares
}

func testRecipients(t *testing.T) ([3]string, [3]*age.X25519Identity) {
	t.Helper()
	var recipients [3]string
	var identities [3]*age.X25519Identity
	for i := range identities {
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatalf("generate identity: %v", err)
		}
		identities[i] = identity
		recipients[i] = identity.Recipient().String()
	}
	return recipients, identities
}
