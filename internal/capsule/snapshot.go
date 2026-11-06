package capsule

import (
	"errors"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

// ValidateSnapshot verifies capsule records and their private state in one read transaction.
func (s *Store) validateSnapshot(tx *bolt.Tx) error {
	if s == nil || tx == nil {
		return errors.New("capsule snapshot validator requires a store and transaction")
	}
	records := tx.Bucket([]byte(installation.CapsulesBucket))
	keys := tx.Bucket([]byte(installation.CapsuleKeysBucket))
	if records == nil && keys == nil {
		return nil
	}
	if records == nil || keys == nil {
		return ErrIntegrity
	}
	seen := make(map[string]struct{})
	if err := records.ForEach(func(id, data []byte) error {
		record, err := decodeRecord(string(id), data)
		if err != nil || (s.plaintextLimit > 0 && record.PlaintextBytes > s.plaintextLimit) {
			return ErrIntegrity
		}
		key := keys.Get(id)
		if len(key) != 32 {
			return ErrIntegrity
		}
		var retained [32]byte
		copy(retained[:], key)
		if err := authenticateDraft(record, retained); err != nil {
			return ErrIntegrity
		}
		seen[string(id)] = struct{}{}
		return nil
	}); err != nil {
		return err
	}
	return keys.ForEach(func(id, _ []byte) error {
		if _, ok := seen[string(id)]; !ok {
			return ErrIntegrity
		}
		return nil
	})
}

// ValidateSnapshot verifies capsule state without opening or changing the database.
func ValidateSnapshot(tx *bolt.Tx, plaintextLimit int) error {
	if _, err := EncodedObjectLimit(plaintextLimit); err != nil {
		return err
	}
	s := &Store{plaintextLimit: plaintextLimit}
	return s.validateSnapshot(tx)
}
