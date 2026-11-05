// Package owner provides the local owner identity and credentials.
package owner

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"

	"golang.org/x/crypto/argon2"
)

var hashSlots = make(chan struct{}, 2)

// Password records the versioned Argon2id profile and its result.
type Password struct {
	Algorithm  string `json:"algorithm"`
	Version    int    `json:"version"`
	Memory     uint32 `json:"memory_kib"`
	Iterations uint32 `json:"iterations"`
	Lanes      uint8  `json:"lanes"`
	Salt       string `json:"salt"`
	Digest     string `json:"digest"`
}

func derive(password, salt []byte) []byte {
	// Two hashes limit the active Argon2 allocation to 128 MiB.
	hashSlots <- struct{}{}
	defer func() { <-hashSlots }()
	return argon2.IDKey(password, salt, 3, 64*1024, 4, 32)
}

func newPassword(password []byte) (Password, error) {
	if len(password) == 0 || len(password) > 1024 {
		return Password{}, errors.New("password requires 1 to 1024 bytes")
	}
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	return Password{"argon2id", argon2.Version, 64 * 1024, 3, 4, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(derive(password, salt))}, nil
}

func (p Password) validate() ([]byte, []byte, error) {
	if p.Algorithm != "argon2id" || p.Version != argon2.Version || p.Memory != 64*1024 || p.Iterations != 3 || p.Lanes != 4 {
		return nil, nil, errors.New("unsupported password profile")
	}
	salt, err := base64.RawStdEncoding.DecodeString(p.Salt)
	if err != nil || len(salt) != 16 {
		return nil, nil, errors.New("invalid password salt")
	}
	digest, err := base64.RawStdEncoding.DecodeString(p.Digest)
	if err != nil || len(digest) != 32 {
		return nil, nil, errors.New("invalid password digest")
	}
	return salt, digest, nil
}

func (p Password) matches(password []byte) bool {
	salt, digest, err := p.validate()
	if err != nil || len(password) == 0 || len(password) > 1024 {
		return false
	}
	return subtle.ConstantTimeCompare(derive(password, salt), digest) == 1
}
