package switches

import (
	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

// ValidateSnapshot verifies main switch records without restoring agent authority.
// Older installations can omit the switch bucket.
func ValidateSnapshot(tx *bolt.Tx) error {
	if tx == nil {
		return ErrInput
	}
	bucket := tx.Bucket([]byte(installation.SwitchesBucket))
	if bucket == nil {
		return nil
	}
	return bucket.ForEach(func(id, data []byte) error {
		if _, err := decode(string(id), data); err != nil {
			return ErrIntegrity
		}
		return nil
	})
}
