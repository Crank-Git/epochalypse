package capsule

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	jose "github.com/go-jose/go-jose/v4"
)

// FormatVersion identifies the authenticated capsule format.
const FormatVersion = 1

// DefaultPlaintextLimit bounds content before encryption.
const DefaultPlaintextLimit = 10 * 1024 * 1024

// ErrLimit identifies content that exceeds the configured size.
var ErrLimit = errors.New("capsule content exceeds its limit")

// ErrObject identifies unsupported or unauthenticated capsule content.
var ErrObject = errors.New("invalid capsule object")

const protectedHeaderLimit = 1024
const objectType = "epochalypse-capsule+jwe"

// EncodedObjectLimit returns the finite compact-object allowance for a plaintext limit.
// Invalid or overflowing limits return ErrLimit.
func EncodedObjectLimit(limit int) (int, error) {
	const overhead = protectedHeaderLimit + 16 + 22 + 4
	maxInt := int(^uint(0) >> 1)
	if limit <= 0 || limit > (maxInt-overhead)/4*3 {
		return 0, ErrLimit
	}
	return encodedLength(limit) + overhead, nil
}

func encodedLength(size int) int {
	return size/3*4 + (size%3*8+5)/6
}

// SealObject returns authenticated content and a separate fresh content key.
// The identifier uses 32 lowercase hexadecimal characters. Unsupported versions return ErrObject.
func SealObject(id string, version int, plaintext []byte, limit int) (string, [32]byte, error) {
	var key [32]byte
	if _, err := expectedHeader(id, version); err != nil {
		return "", key, err
	}
	if _, err := EncodedObjectLimit(limit); err != nil || len(plaintext) > limit {
		return "", key, ErrLimit
	}
	_, _ = rand.Read(key[:])
	options := (&jose.EncrypterOptions{}).
		WithHeader("capsule_id", id).
		WithHeader("capsule_version", version).
		WithHeader("typ", objectType)
	encrypter, err := jose.NewEncrypter(jose.A256GCM, jose.Recipient{Algorithm: jose.DIRECT, Key: key[:]}, options)
	if err != nil {
		return "", [32]byte{}, ErrObject
	}
	sealed, err := encrypter.Encrypt(plaintext)
	if err != nil {
		return "", [32]byte{}, ErrObject
	}
	object, err := sealed.CompactSerialize()
	if err != nil || preflight(id, version, object, limit) != nil {
		return "", [32]byte{}, ErrObject
	}
	return object, key, nil
}

// OpenObject returns plaintext only after complete authentication succeeds.
// Unsupported profiles and authentication failures return ErrObject without plaintext.
func OpenObject(id string, version int, object string, key [32]byte, limit int) ([]byte, error) {
	if err := preflight(id, version, object, limit); err != nil {
		return nil, err
	}
	sealed, err := jose.ParseEncryptedCompact(object, []jose.KeyAlgorithm{jose.DIRECT}, []jose.ContentEncryption{jose.A256GCM})
	if err != nil {
		return nil, ErrObject
	}
	plaintext, err := sealed.Decrypt(key[:])
	if err != nil {
		return nil, ErrObject
	}
	return plaintext, nil
}

func expectedHeader(id string, version int) (string, error) {
	if len(id) != 32 || version != FormatVersion {
		return "", ErrObject
	}
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 16 || strings.ToLower(id) != id {
		return "", ErrObject
	}
	// One canonical header excludes duplicate keys and every unapproved parameter before JSON parsing.
	header := `{"alg":"dir","capsule_id":"` + id + `","capsule_version":1,"enc":"A256GCM","typ":"` + objectType + `"}`
	return base64.RawURLEncoding.EncodeToString([]byte(header)), nil
}

func preflight(id string, version int, object string, limit int) error {
	maximum, err := EncodedObjectLimit(limit)
	if err != nil {
		return err
	}
	if len(object) > maximum {
		return ErrObject
	}
	header, err := expectedHeader(id, version)
	if err != nil {
		return err
	}
	parts, err := compactParts(object)
	if err != nil {
		return err
	}
	if len(parts[0]) > protectedHeaderLimit || parts[0] != header || parts[1] != "" || len(parts[2]) != 16 || len(parts[4]) != 22 {
		return ErrObject
	}
	if len(parts[3]) > encodedLength(limit) {
		return ErrLimit
	}
	for _, part := range parts[2:] {
		if !canonicalBase64(part) {
			return ErrObject
		}
	}
	return nil
}

func compactParts(object string) ([5]string, error) {
	var parts [5]string
	var ok bool
	for i := range 4 {
		parts[i], object, ok = strings.Cut(object, ".")
		if !ok {
			return parts, ErrObject
		}
	}
	if strings.ContainsRune(object, '.') {
		return parts, ErrObject
	}
	parts[4] = object
	return parts, nil
}

func canonicalBase64(segment string) bool {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := 0
	for i := range len(segment) {
		last = strings.IndexByte(alphabet, segment[i])
		if last < 0 {
			return false
		}
	}
	switch len(segment) % 4 {
	case 1:
		return false
	case 2:
		return last&15 == 0
	case 3:
		return last&3 == 0
	default:
		return true
	}
}
