package countdown

import (
	"errors"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

// ValidateSnapshot verifies every retained countdown in one read transaction.
func (s *Store) validateSnapshot(tx *bolt.Tx) error {
	if s == nil || tx == nil {
		return errors.New("countdown snapshot validator requires a store and transaction")
	}
	bucket := tx.Bucket([]byte(installation.CountdownsBucket))
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

// ValidateSnapshot verifies countdown state without opening or changing the database.
func ValidateSnapshot(tx *bolt.Tx) error {
	return (&Store{}).validateSnapshot(tx)
}
