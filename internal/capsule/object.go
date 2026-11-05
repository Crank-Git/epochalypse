package capsule

import "errors"

const FormatVersion = 1
const DefaultPlaintextLimit = 10 * 1024 * 1024

var ErrLimit = errors.New("capsule content exceeds its limit")
var ErrObject = errors.New("invalid capsule object")

func SealObject(id string, version int, plaintext []byte, limit int) (string, [32]byte, error) {
	return "", [32]byte{}, ErrObject
}

func OpenObject(id string, version int, object string, key [32]byte, limit int) ([]byte, error) {
	return nil, ErrObject
}
