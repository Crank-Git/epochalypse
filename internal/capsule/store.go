package capsule

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"
	"unicode/utf8"

	"github.com/Crank-Git/epochalypse/internal/installation"

	bolt "go.etcd.io/bbolt"
)

// Store shares retained content transactions and keeps keys outside capsule records.
type Store struct {
	db             *installation.Handle
	owned          *installation.Store
	plaintextLimit int
}

// PlaintextLimit returns the configured allowance for new content.
func (s *Store) PlaintextLimit() int { return s.plaintextLimit }

// Open owns the installation lifetime for an offline capsule command.
func Open(directory string, limits installation.Limits, plaintextLimit int) (*Store, error) {
	shared, err := installation.Open(directory, limits)
	if err != nil {
		return nil, err
	}
	store, err := Attach(shared, plaintextLimit)
	if err != nil {
		_ = shared.Close()
		return nil, err
	}
	store.owned = shared
	return store, nil
}

// Attach shares an installation handle without ownership of its lifetime.
func Attach(shared *installation.Store, plaintextLimit int) (*Store, error) {
	if shared == nil {
		return nil, ErrInput
	}
	if _, err := EncodedObjectLimit(plaintextLimit); err != nil {
		return nil, err
	}
	s := &Store{db: shared.Handle(), plaintextLimit: plaintextLimit}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{installation.CapsulesBucket, installation.CapsuleKeysBucket} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s, nil
}

// Close stops capsule access and releases the database only when Open owns it.
func (s *Store) Close() error {
	_ = s.db.Close()
	if s.owned != nil {
		return s.owned.Close()
	}
	return nil
}

// Create persists one private draft after bounded content preparation.
func (s *Store) Create(input Input, content io.Reader, now time.Time) (Record, error) {
	now = now.UTC().Truncate(time.Second)
	input, err := prepareInput(input, now)
	if err != nil {
		return Record{}, err
	}
	if content == nil {
		return Record{}, ErrInput
	}
	plaintext, err := io.ReadAll(io.LimitReader(content, int64(s.plaintextLimit)+1))
	defer clear(plaintext)
	if err != nil {
		// Reader failures can contain plaintext or paths, so this error excludes the source error.
		return Record{}, ErrInput
	}
	if len(plaintext) > s.plaintextLimit {
		return Record{}, ErrLimit
	}
	if input.Kind == Text && !utf8.Valid(plaintext) {
		return Record{}, ErrInput
	}
	record, key, err := prepareRecord(input, plaintext, now, s.plaintextLimit)
	defer clear(key[:])
	if err != nil {
		return Record{}, err
	}
	encoded, err := encodeRecord(record)
	if err != nil {
		return Record{}, err
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(installation.CapsulesBucket))
		keys := tx.Bucket([]byte(installation.CapsuleKeysBucket))
		if bucket == nil || keys == nil {
			return ErrIntegrity
		}
		if bucket.Get([]byte(record.ID)) != nil || keys.Get([]byte(record.ID)) != nil {
			return ErrIntegrity
		}
		if err := s.db.Admit(tx, 1, int64(len(encoded))+32); err != nil {
			return err
		}
		if err := bucket.Put([]byte(record.ID), encoded); err != nil {
			return err
		}
		return keys.Put([]byte(record.ID), key[:])
	})
	if err != nil {
		return Record{}, fmt.Errorf("persist capsule draft: %w", err)
	}
	return record, nil
}

// Get returns an owner record independently of publication.
func (s *Store) Get(id string) (Record, error) {
	if _, err := expectedHeader(id, FormatVersion); err != nil {
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

// Published returns an object only after owner publication.
func (s *Store) Published(id string) (Record, error) {
	record, err := s.Get(id)
	if err != nil {
		return Record{}, err
	}
	if record.PublishedAt == nil {
		return Record{}, ErrNotFound
	}
	return record, nil
}

// Publish commits publication after mode-specific draft verification.
func (s *Store) Publish(id string, now time.Time) (Record, error) {
	if _, err := expectedHeader(id, FormatVersion); err != nil {
		return Record{}, ErrInput
	}
	now = now.UTC().Truncate(time.Second)
	if !validStoredTime(now) {
		return Record{}, ErrInput
	}
	var published Record
	err := s.db.Update(func(tx *bolt.Tx) error {
		record, err := recordAt(tx, id)
		if err != nil {
			return err
		}
		if record.PublishedAt != nil {
			published = record
			return nil
		}
		if now.Before(record.CreatedAt) {
			return ErrInput
		}
		key, err := keyAt(tx, id)
		if err != nil {
			return err
		}
		if err := authenticateDraft(record, key); err != nil {
			return err
		}
		record.PublishedAt = &now
		if record.Mode == Public {
			record.EffectiveReleaseAt = &now
		}
		if err := s.replace(tx, record); err != nil {
			return err
		}
		published = record
		return nil
	})
	if err != nil {
		return Record{}, fmt.Errorf("publish capsule: %w", err)
	}
	return published, nil
}

// Key returns a published key after the selected release condition permits it.
func (s *Store) Key(id string, now time.Time) ([32]byte, error) {
	if _, err := expectedHeader(id, FormatVersion); err != nil {
		return [32]byte{}, ErrInput
	}
	if now.IsZero() {
		return [32]byte{}, ErrInput
	}
	var key [32]byte
	err := s.db.View(func(tx *bolt.Tx) error {
		record, err := recordAt(tx, id)
		if err != nil {
			return err
		}
		if record.PublishedAt == nil {
			return ErrNotFound
		}
		if record.Mode != Public && now.Before(*record.EffectiveReleaseAt) {
			return ErrTooEarly
		}
		if record.Mode == Tlock {
			return ErrUnavailable
		}
		candidate, err := keyAt(tx, id)
		if err != nil {
			return err
		}
		if err := authenticate(record, candidate); err != nil {
			return err
		}
		key = candidate
		return nil
	})
	return key, err
}

func prepareRecord(input Input, plaintext []byte, now time.Time, limit int) (Record, [32]byte, error) {
	var random [16]byte
	_, _ = rand.Read(random[:])
	id := hex.EncodeToString(random[:])
	object, key, err := SealObject(id, FormatVersion, plaintext, limit)
	defer clear(key[:])
	if err != nil {
		return Record{}, [32]byte{}, err
	}
	manifest := Manifest{Version: FormatVersion, ID: id, Mode: input.Mode, Kind: input.Kind, Title: input.Title, Filename: input.Filename, CreatedAt: now, RequestedReleaseAt: input.ReleaseAt, PlaintextBytes: len(plaintext), ObjectSHA256: objectDigest(object)}
	if input.Mode == ServerHeld {
		manifest.EffectiveReleaseAt = &input.ReleaseAt
	}
	record := Record{Manifest: manifest, Object: object}
	if input.Mode == Tlock {
		return prepareTimelock(record, key)
	}
	return record, key, nil
}

func prepareTimelock(record Record, key [32]byte) (Record, [32]byte, error) {
	defer clear(key[:])
	if err := authenticate(record, key); err != nil {
		return Record{}, [32]byte{}, err
	}
	wrapped, err := WrapKey(key, record.RequestedReleaseAt)
	if err != nil {
		return Record{}, [32]byte{}, err
	}
	record.Timelock = &wrapped
	record.EffectiveReleaseAt = &wrapped.EffectiveAt
	proof, err := preparationProof(record)
	return record, proof, err
}

func (s *Store) replace(tx *bolt.Tx, record Record) error {
	encoded, err := encodeRecord(record)
	if err != nil {
		return err
	}
	bucket := tx.Bucket([]byte(installation.CapsulesBucket))
	if err := bucket.Delete([]byte(record.ID)); err != nil {
		return err
	}
	// The transaction restores the draft if the larger published record exceeds the shared budget.
	if err := s.db.Admit(tx, 1, int64(len(encoded))); err != nil {
		return err
	}
	return bucket.Put([]byte(record.ID), encoded)
}
