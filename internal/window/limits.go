package window

import (
	"errors"
	bolt "go.etcd.io/bbolt"
)

// Limits bounds retained windows and observation runs in one installation budget.
type Limits struct {
	MaxObjects int
	MaxBytes   int64
}

// ErrQuota identifies a rejected creation that exceeds the retained-state budget.
var ErrQuota = errors.New("window retention quota exceeded")

// DefaultLimits returns the installation defaults for retained public documents.
func DefaultLimits() Limits { return Limits{MaxObjects: 100, MaxBytes: 1 << 30} }

// Validate rejects nonpositive retention limits.
func (l Limits) Validate() error {
	if l.MaxObjects <= 0 || l.MaxBytes <= 0 {
		return errors.New("retention limits must be positive")
	}
	return nil
}

// OpenWithLimits opens window state with explicit positive retention limits.
func OpenWithLimits(stateDir string, signer *Signer, limits Limits) (*Store, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	return openStore(stateDir, signer, limits)
}

func (l Limits) admits(tx *bolt.Tx, size int) error {
	used := int64(size)
	count := 1
	if used > l.MaxBytes {
		return ErrQuota
	}
	for _, name := range [][]byte{windowBucket, observationBucket} {
		err := tx.Bucket(name).ForEach(func(_, value []byte) error {
			count++
			retained := len(value)
			if string(name) == string(observationBucket) {
				var err error
				retained, err = observationBytes(value)
				if err != nil {
					return err
				}
			}
			used += int64(retained)
			if used > l.MaxBytes || count > l.MaxObjects {
				return ErrQuota
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
