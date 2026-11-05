package owner

import (
	"errors"

	bolt "go.etcd.io/bbolt"
)

// ValidateSnapshot checks the owner identity without a database mutation.
// The caller supplies a read transaction from a private, authenticated snapshot.
func ValidateSnapshot(tx *bolt.Tx) error {
	if tx.Bucket(stateBucket) == nil {
		return errors.New("snapshot requires owner state")
	}
	_, err := readIdentity(tx)
	return err
}
