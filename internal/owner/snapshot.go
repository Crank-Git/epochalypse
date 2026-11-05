package owner

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"

	bolt "go.etcd.io/bbolt"
)

// ValidateSnapshot checks every version 1 owner record without a database mutation.
// The caller supplies a read transaction from a private, authenticated snapshot.
// Expired credentials remain valid stored records and retain their original deadlines.
func ValidateSnapshot(tx *bolt.Tx) error {
	names := [][]byte{stateBucket, sessionBucket, tokenBucket, auditBucket}
	if err := tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
		if !slices.ContainsFunc(names, func(allowed []byte) bool { return bytes.Equal(name, allowed) }) {
			return errors.New("snapshot contains an unsupported owner bucket")
		}
		return nil
	}); err != nil {
		return err
	}
	for _, name := range names {
		if tx.Bucket(name) == nil {
			return errors.New("snapshot requires every owner bucket")
		}
	}
	// Check the byte limit before the identity parser allocates from stored JSON.
	if err := tx.Bucket(stateBucket).ForEach(validateSnapshotIdentity); err != nil {
		return err
	}
	identity, err := readIdentity(tx)
	if err != nil {
		return err
	}
	if !validTokenID(identity.ID) {
		return errors.New("snapshot requires a hexadecimal owner identifier")
	}
	checks := []func([]byte, []byte) error{
		func(key, data []byte) error { return validateSnapshotSession(key, data, identity.ID) },
		func(key, data []byte) error { return validateSnapshotToken(key, data, identity.ID) },
		func(key, data []byte) error {
			return validateSnapshotAudit(key, data, tx.Bucket(auditBucket).Sequence())
		},
	}
	for i, name := range names[1:] {
		if err := tx.Bucket(name).ForEach(checks[i]); err != nil {
			return err
		}
	}
	return nil
}

func snapshotDocument(data []byte, dst any) error {
	if len(data) == 0 || len(data) > 16384 {
		return errors.New("snapshot requires a bounded owner record")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errors.New("snapshot contains an invalid owner record")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("snapshot requires one owner record document")
	}
	return nil
}

func validateSnapshotIdentity(key, data []byte) error {
	var record Identity
	if !bytes.Equal(key, identityKey) {
		return errors.New("snapshot contains an unexpected owner record")
	}
	return snapshotDocument(data, &record)
}

func validateSnapshotSession(key, data []byte, ownerID string) error {
	var record session
	if err := snapshotDocument(data, &record); err != nil {
		return err
	}
	if len(key) != 32 || record.OwnerID != ownerID || !record.valid(record.Seen) {
		return errors.New("snapshot contains invalid session state")
	}
	return nil
}

func validateSnapshotToken(key, data []byte, ownerID string) error {
	var record tokenRecord
	if err := snapshotDocument(data, &record); err != nil {
		return err
	}
	if len(key) != 32 || record.Digest != hex.EncodeToString(key) || !record.valid(ownerID, record.CreatedAt) {
		return errors.New("snapshot contains invalid token state")
	}
	return nil
}

func validateSnapshotAudit(key, data []byte, sequence uint64) error {
	var record AuditEvent
	if err := snapshotDocument(data, &record); err != nil {
		return err
	}
	if len(key) != 8 || !record.valid() {
		return errors.New("snapshot contains invalid audit state")
	}
	id := binary.BigEndian.Uint64(key)
	if id == 0 || id > sequence || sequence-id >= auditCapacity {
		return errors.New("snapshot contains an invalid audit sequence")
	}
	return nil
}
