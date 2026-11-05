package capsule

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

func objectDigest(object string) string {
	digest := sha256.Sum256([]byte(object))
	return hex.EncodeToString(digest[:])
}

func encodeRecord(record Record) ([]byte, error) {
	data, err := json.Marshal(record)
	if err != nil || len(data)-len(record.Object) > metadataAllowance {
		return nil, ErrIntegrity
	}
	return data, nil
}

func decodeRecord(id string, data []byte) (Record, error) {
	if data == nil {
		return Record{}, ErrNotFound
	}
	var manifest Manifest
	// The first pass skips the object so its declared plaintext size can bound object allocation.
	if json.Unmarshal(data, &manifest) != nil || manifest.ID != id || validateManifest(manifest) != nil {
		return Record{}, ErrIntegrity
	}
	maximum, err := EncodedObjectLimit(max(1, manifest.PlaintextBytes))
	if err != nil || maximum > int(^uint(0)>>1)-metadataAllowance || len(data) > maximum+metadataAllowance {
		return Record{}, ErrIntegrity
	}
	var record Record
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil {
		return Record{}, ErrIntegrity
	}
	if err := validateRecordTimelock(record); err != nil {
		return Record{}, err
	}
	if record.ObjectSHA256 != objectDigest(record.Object) || preflight(id, record.Version, record.Object, max(1, record.PlaintextBytes)) != nil {
		return Record{}, ErrIntegrity
	}
	parts, err := compactParts(record.Object)
	if err != nil || len(parts[3]) != encodedLength(record.PlaintextBytes) {
		return Record{}, ErrIntegrity
	}
	return record, nil
}

func validateRecordTimelock(record Record) error {
	if record.Mode != Tlock {
		if record.Timelock != nil {
			return ErrIntegrity
		}
		return nil
	}
	if record.Timelock == nil || !record.Timelock.RequestedAt.Equal(record.RequestedReleaseAt) || !record.Timelock.EffectiveAt.Equal(*record.EffectiveReleaseAt) {
		return ErrIntegrity
	}
	_, err := record.Timelock.validate()
	return err
}

func preparationProof(record Record) ([32]byte, error) {
	// Publication changes one field; the proof binds every immutable draft field.
	record.PublishedAt = nil
	data, err := encodeRecord(record)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}

func authenticateDraft(record Record, retained [32]byte) error {
	if record.Mode != Tlock {
		return authenticate(record, retained)
	}
	proof, err := preparationProof(record)
	if err != nil || proof != retained {
		return ErrIntegrity
	}
	return nil
}

func recordAt(tx *bolt.Tx, id string) (Record, error) {
	bucket := tx.Bucket([]byte(installation.CapsulesBucket))
	if bucket == nil {
		return Record{}, ErrIntegrity
	}
	return decodeRecord(id, bucket.Get([]byte(id)))
}

func keyAt(tx *bolt.Tx, id string) ([32]byte, error) {
	bucket := tx.Bucket([]byte(installation.CapsuleKeysBucket))
	if bucket == nil {
		return [32]byte{}, ErrUnavailable
	}
	data := bucket.Get([]byte(id))
	if len(data) != 32 {
		return [32]byte{}, ErrUnavailable
	}
	return [32]byte(data), nil
}

func authenticate(record Record, key [32]byte) error {
	plaintext, err := OpenObject(record.ID, record.Version, record.Object, key, max(1, record.PlaintextBytes))
	defer clear(plaintext)
	if err != nil || len(plaintext) != record.PlaintextBytes {
		return ErrIntegrity
	}
	return nil
}
