package installation

import (
	"errors"

	bolt "go.etcd.io/bbolt"
)

// WindowsBucket and the other names preserve the version 1 content layout.
const (
	WindowsBucket       = "windows-v1"
	ObservationsBucket  = "observations-v1"
	CapsulesBucket      = "capsules-v1"
	CapsuleKeysBucket   = "capsule-keys-v1"
	CountdownsBucket    = "countdowns-v1"
	ConfigurationBucket = "configuration-v1"
)

// ValidateSnapshotLayout rejects buckets that the current format cannot restore.
func ValidateSnapshotLayout(tx *bolt.Tx) error {
	if tx == nil {
		return errors.New("snapshot transaction is required")
	}
	seen := make(map[string]struct{}, 6)
	if err := tx.ForEach(func(name []byte, bucket *bolt.Bucket) error {
		if string(name) != ConfigurationBucket && !contentBucket(string(name)) {
			return errors.New("snapshot contains an unsupported bucket")
		}
		seen[string(name)] = struct{}{}
		return bucket.ForEach(func(_, value []byte) error {
			if value == nil {
				return errors.New("snapshot contains a nested bucket")
			}
			return nil
		})
	}); err != nil {
		return err
	}
	for _, name := range []string{WindowsBucket, ObservationsBucket, CapsulesBucket, CapsuleKeysBucket, CountdownsBucket, ConfigurationBucket} {
		if _, ok := seen[name]; !ok {
			return errors.New("snapshot omits a required bucket")
		}
	}
	return nil
}
