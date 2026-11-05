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
	if record.ObjectSHA256 != objectDigest(record.Object) || preflight(id, record.Version, record.Object, max(1, record.PlaintextBytes)) != nil {
		return Record{}, ErrIntegrity
	}
	parts, err := compactParts(record.Object)
	if err != nil || len(parts[3]) != encodedLength(record.PlaintextBytes) {
		return Record{}, ErrIntegrity
	}
	return record, nil
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
	if err != nil || len(plaintext) != record.PlaintextBytes {
		return ErrIntegrity
	}
	return nil
}
