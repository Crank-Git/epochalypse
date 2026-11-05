package capsule

import (
	"context"
	"io"
	"time"
)

// RecoveryBundle contains versioned public recovery data and an optional released key.
type RecoveryBundle struct {
	Version     int    `json:"version"`
	Record      Record `json:"record"`
	ReleasedKey string `json:"released_key,omitempty"`
}

// Bundle returns recovery data only for a published capsule.
func (s *Store) Bundle(id string, now time.Time) (RecoveryBundle, error) {
	return RecoveryBundle{}, ErrUnavailable
}

// Open authenticates the complete content before it returns plaintext.
func (b RecoveryBundle) Open(ctx context.Context, source BeaconSource, now time.Time) ([]byte, error) {
	return nil, ErrUnavailable
}

// DecodeBundle returns a bounded canonical recovery bundle.
func DecodeBundle(input io.Reader, limit int) (RecoveryBundle, error) {
	return RecoveryBundle{}, ErrIntegrity
}
