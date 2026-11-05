package capsule

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

// ImportPrepared persists a structurally validated tlock draft without an unwrapped key.
func (s *Store) ImportPrepared(ctx context.Context, content io.Reader, now time.Time) (Record, error) {
	now = now.UTC().Truncate(time.Second)
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	record, err := decodePrepared(content, s.plaintextLimit, now)
	if err != nil {
		return Record{}, err
	}
	proof, err := preparationProof(record)
	defer clear(proof[:])
	if err != nil {
		return Record{}, err
	}
	encoded, err := encodeRecord(record)
	if err != nil {
		return Record{}, err
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		bucket := tx.Bucket([]byte(installation.CapsulesBucket))
		keys := tx.Bucket([]byte(installation.CapsuleKeysBucket))
		if bucket == nil || keys == nil || bucket.Get([]byte(record.ID)) != nil || keys.Get([]byte(record.ID)) != nil {
			return ErrIntegrity
		}
		if err := s.db.Admit(tx, 1, int64(len(encoded))+32); err != nil {
			return err
		}
		if err := bucket.Put([]byte(record.ID), encoded); err != nil {
			return err
		}
		if err := keys.Put([]byte(record.ID), proof[:]); err != nil {
			return err
		}
		return ctx.Err()
	})
	if err != nil {
		return Record{}, err
	}
	return record, nil
}

func decodePrepared(content io.Reader, limit int, now time.Time) (Record, error) {
	maximum, err := EncodedObjectLimit(limit)
	if err != nil || content == nil || !validStoredTime(now) {
		return Record{}, ErrInput
	}
	data, err := io.ReadAll(io.LimitReader(content, int64(maximum)+metadataAllowance+1))
	if err != nil {
		return Record{}, ErrInput
	}
	if int64(len(data)) > int64(maximum)+metadataAllowance {
		return Record{}, ErrLimit
	}
	var manifest Manifest
	if json.Unmarshal(data, &manifest) != nil || manifest.Mode != Tlock || manifest.PublishedAt != nil || manifest.CreatedAt.After(now) || manifest.RequestedReleaseAt.Before(now) {
		return Record{}, ErrInput
	}
	if manifest.PlaintextBytes > limit {
		return Record{}, ErrLimit
	}
	record, err := decodeRecord(manifest.ID, data)
	if err != nil {
		return Record{}, err
	}
	canonical, err := encodeRecord(record)
	if err != nil || !bytes.Equal(data, canonical) {
		return Record{}, ErrIntegrity
	}
	return record, nil
}
