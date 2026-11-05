package switches

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

// Store shares the installation database and retention allowance.
type Store struct {
	db *installation.Handle
}

// Attach keeps the shared database lifetime with the caller.
func Attach(shared *installation.Store) (*Store, error) {
	if shared == nil {
		return nil, ErrInput
	}
	s := &Store{db: shared.Handle()}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(installation.SwitchesBucket))
		return err
	}); err != nil {
		return nil, err
	}
	return s, nil
}

// Close stops this domain without stopping other installation domains.
func (s *Store) Close() error { return s.db.Close() }

// Create retains one unarmed draft after atomic admission across all domains.
func (s *Store) Create(ctx context.Context, input Input, now time.Time) (Record, error) {
	interval, grace, err := input.durations()
	if err != nil || now.IsZero() {
		return Record{}, ErrInput
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Record{}, err
	}
	record := Record{
		Version:         1,
		ID:              hex.EncodeToString(random[:]),
		IntervalSeconds: interval,
		GraceSeconds:    grace,
		CreatedAt:       now.UTC().Truncate(time.Second),
		State:           StateDraft,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return Record{}, err
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		bucket := tx.Bucket([]byte(installation.SwitchesBucket))
		if bucket == nil || bucket.Get([]byte(record.ID)) != nil {
			return ErrIntegrity
		}
		if err := s.db.Admit(tx, 1, int64(len(encoded))); err != nil {
			return err
		}
		if err := bucket.Put([]byte(record.ID), encoded); err != nil {
			return err
		}
		return ctx.Err()
	})
	if err != nil {
		return Record{}, err
	}
	return record, nil
}

// Get returns the retained configuration for one draft.
func (s *Store) Get(id string) (Record, error) {
	if !validID(id) {
		return Record{}, ErrInput
	}
	var record Record
	err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		record, err = recordAt(tx, id)
		return err
	})
	return record, err
}

// List returns a snapshot of retained drafts.
func (s *Store) List() ([]Record, error) {
	list := []Record{}
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(installation.SwitchesBucket))
		if bucket == nil {
			return ErrIntegrity
		}
		return bucket.ForEach(func(id, data []byte) error {
			record, err := decode(string(id), data)
			if err != nil {
				return err
			}
			list = append(list, record)
			return nil
		})
	})
	return list, err
}

func recordAt(tx *bolt.Tx, id string) (Record, error) {
	bucket := tx.Bucket([]byte(installation.SwitchesBucket))
	if bucket == nil {
		return Record{}, ErrIntegrity
	}
	return decode(id, bucket.Get([]byte(id)))
}

func decode(id string, data []byte) (Record, error) {
	if data == nil {
		return Record{}, ErrNotFound
	}
	if len(data) > MaxRecordBytes {
		return Record{}, ErrIntegrity
	}
	var record Record
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || decoder.Decode(&struct{}{}) != io.EOF || record.ID != id || ValidateRecord(record) != nil {
		return Record{}, ErrIntegrity
	}
	return record, nil
}
