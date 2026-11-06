package installation

import (
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Limits bounds all retained content in one installation.
type Limits struct {
	MaxObjects int
	MaxBytes   int64
}

// ErrQuota identifies content that exceeds the installation allowance.
var ErrQuota = errors.New("retained state quota exceeded")

// ObservationReservationBytes preserves the version 1 pending-run allowance.
const ObservationReservationBytes = 16384

// DefaultLimits returns the installation allowance for retained content.
func DefaultLimits() Limits { return Limits{MaxObjects: 100, MaxBytes: 1 << 30} }

// Validate rejects nonpositive retention limits.
func (l Limits) Validate() error {
	if l.MaxObjects <= 0 || l.MaxBytes <= 0 {
		return errors.New("retention limits must be positive")
	}
	return nil
}

// Admit checks all retained domains inside the caller's write transaction.
// The additions must be nonnegative. A failed transaction preserves all prior state.
func (s *Store) Admit(tx *bolt.Tx, objects int, size int64) error {
	if err := s.Limits.Validate(); err != nil {
		return err
	}
	if objects < 0 || size < 0 {
		return errors.New("retained additions must be nonnegative")
	}
	if objects > s.Limits.MaxObjects || size > s.Limits.MaxBytes {
		return ErrQuota
	}
	remaining := Limits{MaxObjects: s.Limits.MaxObjects - objects, MaxBytes: s.Limits.MaxBytes - size}
	return tx.ForEach(func(name []byte, bucket *bolt.Bucket) error {
		if string(name) == ConfigurationBucket {
			return nil
		}
		if !contentBucket(string(name)) {
			return errors.New("unknown retained content bucket")
		}
		return bucket.ForEach(func(_, value []byte) error {
			return charge(&remaining, string(name), value)
		})
	})
}

func contentBucket(name string) bool {
	return name == WindowsBucket || name == ObservationsBucket || name == CapsulesBucket || name == CapsuleKeysBucket || name == CountdownsBucket || name == SwitchesBucket
}

func charge(remaining *Limits, name string, value []byte) error {
	if value == nil {
		return errors.New("retained content cannot contain a nested bucket")
	}
	if int64(len(value)) > remaining.MaxBytes {
		return ErrQuota
	}
	size := int64(len(value))
	if name == CapsuleKeysBucket {
		if len(value) != 32 {
			return errors.New("invalid retained capsule key size")
		}
	} else {
		if remaining.MaxObjects == 0 {
			return ErrQuota
		}
		remaining.MaxObjects--
		var version struct {
			Version int `json:"version"`
		}
		if name == ObservationsBucket {
			reserved, err := observationReservation(value)
			if err != nil {
				return err
			}
			size = max(size, reserved)
		} else if json.Unmarshal(value, &version) != nil || version.Version != 1 {
			return errors.New("invalid retained content version")
		}
	}
	if size > remaining.MaxBytes {
		return ErrQuota
	}
	remaining.MaxBytes -= size
	return nil
}

func observationReservation(value []byte) (int64, error) {
	var doc struct {
		Run struct {
			Version   int        `json:"version"`
			State     string     `json:"state"`
			Completed *time.Time `json:"completed"`
		} `json:"run"`
		ReservedBytes int64 `json:"reserved_bytes"`
	}
	if json.Unmarshal(value, &doc) != nil || doc.Run.Version != 1 {
		return 0, errors.New("invalid retained observation version")
	}
	switch doc.Run.State {
	case "pending":
		if doc.ReservedBytes != ObservationReservationBytes || doc.Run.Completed != nil {
			return 0, errors.New("invalid retained observation reservation")
		}
	case "complete", "partial":
		if doc.ReservedBytes != 0 || doc.Run.Completed == nil {
			return 0, errors.New("invalid retained observation state")
		}
	default:
		return 0, errors.New("unknown retained observation state")
	}
	return doc.ReservedBytes, nil
}
