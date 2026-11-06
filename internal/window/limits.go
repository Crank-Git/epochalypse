package window

import (
	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

// Limits bounds retained windows and observation runs in one installation budget.
type Limits = installation.Limits

// ErrQuota identifies a rejected creation that exceeds the retained-state budget.
var ErrQuota = installation.ErrQuota

// DefaultLimits returns the installation defaults for retained public documents.
func DefaultLimits() Limits { return installation.DefaultLimits() }

// OpenWithLimits opens window state with explicit positive retention limits.
func OpenWithLimits(stateDir string, signer *Signer, limits Limits) (*Store, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	return openStore(stateDir, signer, limits)
}

func (s *Store) admits(tx *bolt.Tx, size int) error {
	if err := tx.Bucket(observationBucket).ForEach(func(key, value []byte) error {
		_, err := s.runAt(tx, key, value)
		return err
	}); err != nil {
		return err
	}
	return s.db.Admit(tx, 1, int64(size))
}
