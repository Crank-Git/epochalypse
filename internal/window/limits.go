package window

import (
	"errors"
	bolt "go.etcd.io/bbolt"
)

// Limits bounds retained public window documents.
type Limits struct {
	MaxObjects int
	MaxBytes   int64
}

// ErrQuota identifies a rejected creation that exceeds the retained-state budget.
var ErrQuota = errors.New("window retention quota exceeded")

// DefaultLimits returns the current installation defaults for retained windows.
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

func (l Limits) admits(bucket *bolt.Bucket, size int) error {
	if bucket.Stats().KeyN >= l.MaxObjects {
		return ErrQuota
	}
	used := int64(size)
	if used > l.MaxBytes {
		return ErrQuota
	}
	return bucket.ForEach(func(_, value []byte) error {
		used += int64(len(value))
		if used > l.MaxBytes {
			return ErrQuota
		}
		return nil
	})
}
