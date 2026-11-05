package capsule

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"time"

	bolt "go.etcd.io/bbolt"
)

// RecoveryBundle contains versioned public recovery data and an optional released key.
type RecoveryBundle struct {
	Version     int    `json:"version"`
	Record      Record `json:"record"`
	ReleasedKey string `json:"released_key,omitempty"`
}

// Bundle returns recovery data only for a published capsule.
func (s *Store) Bundle(id string, now time.Time) (RecoveryBundle, error) {
	if _, err := expectedHeader(id, FormatVersion); err != nil || !validStoredTime(now) {
		return RecoveryBundle{}, ErrInput
	}
	var bundle RecoveryBundle
	err := s.db.View(func(tx *bolt.Tx) error {
		record, err := recordAt(tx, id)
		if err != nil {
			return err
		}
		if record.PublishedAt == nil {
			return ErrNotFound
		}
		bundle = RecoveryBundle{Version: 1, Record: record}
		if record.Mode == Tlock || (record.Mode == ServerHeld && now.Before(*record.EffectiveReleaseAt)) {
			return nil
		}
		key, err := keyAt(tx, id)
		defer clear(key[:])
		if err != nil {
			return err
		}
		if err := authenticate(record, key); err != nil {
			return err
		}
		bundle.ReleasedKey = base64.RawURLEncoding.EncodeToString(key[:])
		return nil
	})
	if err != nil {
		return RecoveryBundle{}, err
	}
	return bundle, nil
}

// Open authenticates the complete content before it returns plaintext.
func (b RecoveryBundle) Open(ctx context.Context, source BeaconSource, now time.Time) ([]byte, error) {
	if !validStoredTime(now) {
		return nil, ErrInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := b.validate(); err != nil {
		return nil, err
	}
	if b.Record.Mode != Public && now.Before(*b.Record.EffectiveReleaseAt) {
		return nil, ErrTooEarly
	}
	key, err := b.contentKey(ctx, source, now)
	defer clear(key[:])
	if err != nil {
		return nil, err
	}
	plaintext, err := OpenObject(b.Record.ID, b.Record.Version, b.Record.Object, key, max(1, b.Record.PlaintextBytes))
	if err != nil || len(plaintext) != b.Record.PlaintextBytes {
		clear(plaintext)
		return nil, ErrIntegrity
	}
	if err := ctx.Err(); err != nil {
		clear(plaintext)
		return nil, err
	}
	return plaintext, nil
}

func (b RecoveryBundle) contentKey(ctx context.Context, source BeaconSource, now time.Time) ([32]byte, error) {
	if b.Record.Mode == Tlock {
		return b.Record.Timelock.OpenKey(ctx, source, now)
	}
	if b.ReleasedKey == "" {
		return [32]byte{}, ErrUnavailable
	}
	return decodeReleasedKey(b.ReleasedKey)
}

func decodeReleasedKey(encoded string) ([32]byte, error) {
	data, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	defer clear(data)
	if err != nil || len(data) != 32 {
		return [32]byte{}, ErrIntegrity
	}
	return [32]byte(data), nil
}

func (b RecoveryBundle) validate() error {
	r := b.Record
	if b.Version != 1 || r.PublishedAt == nil || validateManifest(r.Manifest) != nil || len(b.ReleasedKey) > 43 || (r.Mode == Tlock && b.ReleasedKey != "") {
		return ErrIntegrity
	}
	if b.ReleasedKey != "" {
		key, err := decodeReleasedKey(b.ReleasedKey)
		clear(key[:])
		if err != nil {
			return ErrIntegrity
		}
	}
	if validateRecordTimelock(r) != nil || preflight(r.ID, r.Version, r.Object, max(1, r.PlaintextBytes)) != nil {
		return ErrIntegrity
	}
	data, err := encodeRecord(r)
	if err != nil {
		return ErrIntegrity
	}
	_, err = decodeRecord(r.ID, data)
	return err
}

// DecodeBundle returns a bounded canonical recovery bundle.
func DecodeBundle(input io.Reader, limit int) (RecoveryBundle, error) {
	maximum, err := EncodedObjectLimit(limit)
	const allowance = metadataAllowance + 256
	if err != nil || maximum > int(^uint(0)>>1)-allowance-1 || input == nil {
		return RecoveryBundle{}, ErrLimit
	}
	data, err := io.ReadAll(io.LimitReader(input, int64(maximum+allowance)+1))
	if err != nil || len(data) > maximum+allowance {
		return RecoveryBundle{}, ErrIntegrity
	}
	var bundle RecoveryBundle
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&bundle) != nil || bundle.Record.PlaintextBytes > limit || bundle.validate() != nil {
		return RecoveryBundle{}, ErrIntegrity
	}
	// Canonical bytes reject duplicate fields and ambiguous envelopes before recovery.
	canonical, err := json.Marshal(bundle)
	if err != nil || !bytes.Equal(data, canonical) {
		return RecoveryBundle{}, ErrIntegrity
	}
	return bundle, nil
}
