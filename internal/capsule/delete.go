package capsule

import (
	"context"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

// Delete removes local content only after exact identifier confirmation.
func (s *Store) Delete(ctx context.Context, id, confirmation string) error {
	if _, err := expectedHeader(id, FormatVersion); err != nil || confirmation != id {
		return ErrInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := recordAt(tx, id); err != nil {
			return err
		}
		keys := tx.Bucket([]byte(installation.CapsuleKeysBucket))
		if keys == nil {
			return ErrIntegrity
		}
		if err := keys.Delete([]byte(id)); err != nil {
			return err
		}
		if err := tx.Bucket([]byte(installation.CapsulesBucket)).Delete([]byte(id)); err != nil {
			return err
		}
		// Cancellation before commit restores the record and private state together.
		return ctx.Err()
	})
}
