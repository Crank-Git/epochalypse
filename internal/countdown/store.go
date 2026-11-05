package countdown

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

// Store shares installation transactions and preserves metadata when DNS is disabled.
type Store struct {
	db    *installation.Handle
	owned *installation.Store
	zone  string
}

// Open owns the installation lifetime for an offline command.
func Open(directory string, limits installation.Limits, zone string) (*Store, error) {
	shared, err := installation.Open(directory, limits)
	if err != nil {
		return nil, err
	}
	store, err := Attach(shared, zone)
	if err != nil {
		_ = shared.Close()
		return nil, err
	}
	store.owned = shared
	return store, nil
}

// Attach keeps the shared database lifetime with the caller.
// An empty zone disables creation and preserves inspection and deletion.
func Attach(shared *installation.Store, zone string) (*Store, error) {
	if shared == nil || (zone != "" && !ValidZone(zone)) {
		return nil, ErrInput
	}
	s := &Store{db: shared.Handle(), zone: zone}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(installation.CountdownsBucket))
		return err
	}); err != nil {
		return nil, err
	}
	return s, nil
}

// Close stops this domain without stopping other installation domains.
func (s *Store) Close() error {
	_ = s.db.Close()
	if s.owned != nil {
		return s.owned.Close()
	}
	return nil
}

// Create commits one event after atomic admission across all retained domains.
func (s *Store) Create(ctx context.Context, in Input, now time.Time) (Record, error) {
	if s.zone == "" {
		return Record{}, ErrUnavailable
	}
	event, err := in.Validate(now)
	if err != nil {
		return Record{}, err
	}
	var random [16]byte
	_, _ = rand.Read(random[:])
	id := hex.EncodeToString(random[:])
	r := Record{Version: 1, ID: id, Title: in.Title, EventAt: event, CreatedAt: now.UTC().Truncate(time.Second), Zone: s.zone, Name: "c" + id + "." + s.zone}
	encoded, err := json.Marshal(r)
	if err != nil {
		return Record{}, err
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		bucket := tx.Bucket([]byte(installation.CountdownsBucket))
		if bucket == nil || bucket.Get([]byte(id)) != nil {
			return ErrIntegrity
		}
		if err := s.db.Admit(tx, 1, int64(len(encoded))); err != nil {
			return err
		}
		return bucket.Put([]byte(id), encoded)
	})
	if err != nil {
		return Record{}, err
	}
	return r, nil
}

// Get returns immutable metadata independently of the current clock or zone configuration.
func (s *Store) Get(id string) (Record, error) {
	if !validID(id) {
		return Record{}, ErrInput
	}
	var r Record
	err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		r, err = recordAt(tx, id)
		return err
	})
	return r, err
}

// List returns a bounded snapshot charged to the installation allowance.
func (s *Store) List() ([]Record, error) {
	list := []Record{}
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(installation.CountdownsBucket))
		if bucket == nil {
			return ErrIntegrity
		}
		return bucket.ForEach(func(id, data []byte) error {
			r, err := decode(string(id), data)
			if err != nil {
				return err
			}
			list = append(list, r)
			return nil
		})
	})
	return list, err
}

// Delete releases the event's exact object and byte charge in one transaction.
func (s *Store) Delete(ctx context.Context, id string) error {
	if !validID(id) {
		return ErrInput
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := recordAt(tx, id); err != nil {
			return err
		}
		return tx.Bucket([]byte(installation.CountdownsBucket)).Delete([]byte(id))
	})
}

func recordAt(tx *bolt.Tx, id string) (Record, error) {
	bucket := tx.Bucket([]byte(installation.CountdownsBucket))
	if bucket == nil {
		return Record{}, ErrIntegrity
	}
	return decode(id, bucket.Get([]byte(id)))
}

func decode(id string, data []byte) (Record, error) {
	if data == nil {
		return Record{}, ErrNotFound
	}
	if len(data) > 4096 {
		return Record{}, ErrIntegrity
	}
	var r Record
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(&struct{}{}) != io.EOF || r.ID != id || r.validate() != nil {
		return Record{}, ErrIntegrity
	}
	return r, nil
}
