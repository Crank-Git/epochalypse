package contract

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"testing"

	"filippo.io/age"

	"github.com/hashicorp/vault/shamir"
)

func TestRecoversSyntheticKeyFromEveryPair(t *testing.T) {
	secret := bytes.Repeat([]byte{42}, 32)
	parts, err := shamir.Split(secret, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]int{{0, 1}, {0, 2}, {1, 2}} {
		recovered, err := shamir.Combine([][]byte{parts[pair[0]], parts[pair[1]]})
		if err != nil || !bytes.Equal(secret, recovered) {
			t.Fatalf("pair %v differs: %v", pair, err)
		}
	}
	for _, p := range parts {
		if len(p) != 33 {
			t.Fatal("share has unexpected length")
		}
		if _, err := shamir.Combine([][]byte{p}); err == nil {
			t.Fatal("one share supplies a recovery path")
		}
	}
	for _, bad := range [][][]byte{nil, {parts[0], parts[0]}, {parts[0], parts[1][:32]}, {{1}, {2}}} {
		if _, err := shamir.Combine(bad); err == nil {
			t.Fatal("accepts malformed shares")
		}
	}
	for _, args := range [][2]int{{1, 2}, {256, 2}, {3, 1}, {256, 256}} {
		if _, err := shamir.Split(secret, args[0], args[1]); err == nil {
			t.Fatal("accepts invalid threshold")
		}
	}
	if _, err := shamir.Split(nil, 3, 2); err == nil {
		t.Fatal("accepts empty secret")
	}
}

func TestDistributesEachShareToItsEnrolledRecipient(t *testing.T) {
	parts, err := shamir.Split(bytes.Repeat([]byte{42}, 32), 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	var identities [3]*age.X25519Identity
	for i := range identities {
		identities[i], err = age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
	}
	agents := enrollmentAgents(t)
	for i, id := range identities {
		agents[i].Recipient = id.Recipient().String()
	}
	enrollment, err := admitEnrollment([32]byte{3}, agents)
	if err != nil {
		t.Fatal(err)
	}
	for i, part := range parts {
		ciphertext := encryptShare(t, part, enrollment.agents[i].Recipient)
		if len(ciphertext) > 4096 {
			t.Fatal("encrypted share exceeds frame limit")
		}
		for j, id := range identities {
			r, err := age.Decrypt(bytes.NewReader(ciphertext), id)
			if j != i {
				if err == nil {
					t.Fatal("another agent decrypts the share")
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(io.LimitReader(r, 34))
			if err != nil || !bytes.Equal(got, part) {
				t.Fatal("recipient share differs")
			}
		}
	}
}

func encryptShare(t *testing.T, part []byte, value string) []byte {
	t.Helper()
	recipient, err := age.ParseX25519Recipient(value)
	if err != nil {
		t.Fatal(err)
	}
	var ciphertext bytes.Buffer
	w, err := age.Encrypt(&ciphertext, recipient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(part); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return ciphertext.Bytes()
}

func TestCorruptShareRequiresAuthenticatedContentCheck(t *testing.T) {
	key := bytes.Repeat([]byte{42}, 32)
	parts, err := shamir.Split(key, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	sealed := aead.Seal(nil, nonce, []byte("synthetic content"), []byte("synthetic manifest"))
	parts[0][0] ^= 1
	wrong, err := shamir.Combine(parts[:2])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(wrong, key) {
		t.Fatal("corrupt share preserves key")
	}
	block, err = aes.NewCipher(wrong)
	if err != nil {
		t.Fatal(err)
	}
	aead, err = cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext, err := aead.Open(nil, nonce, sealed, []byte("synthetic manifest")); err == nil || plaintext != nil {
		t.Fatal("exposes unauthenticated plaintext")
	}
}
