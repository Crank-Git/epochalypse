// Package switchcontent prepares authenticated content and encrypted recovery shares.
package switchcontent

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"filippo.io/age"
	"github.com/hashicorp/vault/shamir"
)

const (
	maxContentBytes       = 10 << 20
	contentKeyBytes       = 32
	keyIDBytes            = 32
	maxShareEnvelopeBytes = 4096
	shareCount            = 3
	shareThreshold        = 2
	shareBytes            = contentKeyBytes + shamir.ShareOverhead
	framePrefixSize       = 5
	frameHeaderSize       = framePrefixSize + keyIDBytes
	frameNonceSize        = 12
	frameOverhead         = frameHeaderSize + frameNonceSize + 16
	frameVersion          = 1
)

var (
	frameMagic       = [4]byte{'E', 'P', 'S', 'C'}
	errContentTooBig = errors.New("switch content exceeds 10 MiB")
	errInvalidFrame  = errors.New("invalid switch content frame")
	errInvalidShare  = errors.New("invalid switch share")
)

// Material contains authenticated ciphertext and one encrypted share per agent.
type Material struct {
	Ciphertext    []byte
	Shares        [shareCount][]byte
	KeyID         [keyIDBytes]byte
	ContentDigest [sha256.Size]byte
}

// Prepare encrypts bounded content and creates three age-encrypted shares.
func Prepare(source io.Reader, recipientStrings [shareCount]string) (Material, error) {
	if source == nil {
		return Material{}, errors.New("content reader is nil")
	}
	recipients, err := parseRecipients(recipientStrings)
	if err != nil {
		return Material{}, err
	}
	plaintext, err := readContent(source)
	if err != nil {
		return Material{}, err
	}
	defer clear(plaintext)

	var key [contentKeyBytes]byte
	// Go 1.27 crypto/rand.Read fills the buffer or terminates the process.
	_, _ = rand.Read(key[:])
	defer clear(key[:])

	var material Material
	_, _ = rand.Read(material.KeyID[:])
	material.Ciphertext, err = encryptContent(plaintext, key[:], material.KeyID)
	if err != nil {
		return Material{}, err
	}
	material.ContentDigest = sha256.Sum256(material.Ciphertext)

	shares, err := shamir.Split(key[:], shareCount, shareThreshold)
	if err != nil {
		return Material{}, fmt.Errorf("split content key: %w", err)
	}
	defer func() {
		for i := range shares {
			clear(shares[i])
		}
	}()
	for i, share := range shares {
		material.Shares[i], err = encryptShare(share, recipients[i])
		if err != nil {
			return Material{}, fmt.Errorf("encrypt share %d: %w", i, err)
		}
	}
	return material, nil
}

// Recover authenticates content with two or three raw shares.
func Recover(ciphertext []byte, shares [][]byte) ([]byte, error) {
	if len(shares) < shareThreshold || len(shares) > shareCount {
		return nil, errors.New("recovery requires two or three shares")
	}
	if err := validateFrame(ciphertext); err != nil {
		return nil, err
	}

	seen := make(map[byte]struct{}, len(shares))
	for _, share := range shares {
		if len(share) != shareBytes {
			return nil, errInvalidShare
		}
		coordinate := share[len(share)-1]
		if _, exists := seen[coordinate]; exists {
			return nil, errors.New("recovery shares are not distinct")
		}
		seen[coordinate] = struct{}{}
	}
	key, err := shamir.Combine(shares)
	if err != nil {
		return nil, fmt.Errorf("combine recovery shares: %w", err)
	}
	defer clear(key)
	if len(key) != contentKeyBytes {
		return nil, errInvalidShare
	}

	nonceEnd := frameHeaderSize + frameNonceSize
	plaintext, err := decryptContent(ciphertext[:frameHeaderSize], ciphertext[frameHeaderSize:nonceEnd], ciphertext[nonceEnd:], key)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// ValidateBinding checks that ciphertext matches its enrolled key identity and digest.
func ValidateBinding(ciphertext []byte, keyID, contentDigest [32]byte) error {
	if err := validateFrame(ciphertext); err != nil {
		return err
	}
	var actualKeyID [keyIDBytes]byte
	copy(actualKeyID[:], ciphertext[framePrefixSize:frameHeaderSize])
	if actualKeyID != keyID {
		return errors.New("switch content key identity mismatch")
	}
	if sha256.Sum256(ciphertext) != contentDigest {
		return errors.New("switch content digest mismatch")
	}
	return nil
}

// DecryptShare decrypts one share with its enrolled age identity.
func DecryptShare(ciphertext []byte, identity *age.X25519Identity) ([]byte, error) {
	if identity == nil {
		return nil, errors.New("age identity is nil")
	}
	if len(ciphertext) == 0 || len(ciphertext) > maxShareEnvelopeBytes {
		return nil, errInvalidShare
	}
	reader, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
	if err != nil {
		return nil, fmt.Errorf("decrypt share: %w", err)
	}
	plaintext, err := io.ReadAll(io.LimitReader(reader, shareBytes+1))
	if err != nil {
		clear(plaintext)
		return nil, fmt.Errorf("read decrypted share: %w", err)
	}
	if len(plaintext) != shareBytes {
		clear(plaintext)
		return nil, errInvalidShare
	}
	return plaintext, nil
}

func parseRecipients(values [shareCount]string) ([shareCount]*age.X25519Recipient, error) {
	var recipients [shareCount]*age.X25519Recipient
	seen := make(map[string]struct{}, shareCount)
	for i, value := range values {
		recipient, err := age.ParseX25519Recipient(value)
		if err != nil {
			return recipients, fmt.Errorf("parse recipient %d: %w", i, err)
		}
		canonical := recipient.String()
		if canonical != value {
			return recipients, fmt.Errorf("recipient %d is not canonical", i)
		}
		if _, exists := seen[canonical]; exists {
			return recipients, errors.New("age recipients must be distinct")
		}
		seen[canonical] = struct{}{}
		recipients[i] = recipient
	}
	return recipients, nil
}

func validateFrame(ciphertext []byte) error {
	if len(ciphertext) < frameOverhead || len(ciphertext) > maxContentBytes+frameOverhead {
		return errInvalidFrame
	}
	if !bytes.Equal(ciphertext[:4], frameMagic[:]) || ciphertext[4] != frameVersion {
		return errInvalidFrame
	}
	return nil
}

func readContent(source io.Reader) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(source, maxContentBytes+1))
	if err != nil {
		clear(content)
		return nil, fmt.Errorf("read switch content: %w", err)
	}
	if len(content) > maxContentBytes {
		clear(content)
		return nil, errContentTooBig
	}
	return content, nil
}

func encryptContent(plaintext, key []byte, keyID [keyIDBytes]byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create content cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create content authenticator: %w", err)
	}
	header := make([]byte, frameHeaderSize)
	copy(header[:4], frameMagic[:])
	header[4] = frameVersion
	copy(header[framePrefixSize:framePrefixSize+keyIDBytes], keyID[:])
	nonce := make([]byte, aead.NonceSize())
	_, _ = rand.Read(nonce)
	frame := make([]byte, 0, frameOverhead+len(plaintext))
	frame = append(frame, header...)
	frame = append(frame, nonce...)
	frame = aead.Seal(frame, nonce, plaintext, header)
	return frame, nil
}

func decryptContent(header, nonce, sealed, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create content cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create content authenticator: %w", err)
	}
	if len(nonce) != aead.NonceSize() || len(sealed) < aead.Overhead() {
		return nil, errInvalidFrame
	}
	plaintext, err := aead.Open(nil, nonce, sealed, header)
	if err != nil {
		return nil, errors.New("switch content authentication failed")
	}
	return plaintext, nil
}

func encryptShare(share []byte, recipient *age.X25519Recipient) ([]byte, error) {
	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, recipient)
	if err != nil {
		return nil, fmt.Errorf("create age share envelope: %w", err)
	}
	if _, err := io.Copy(writer, bytes.NewReader(share)); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("write age share envelope: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close age share envelope: %w", err)
	}
	return encrypted.Bytes(), nil
}
