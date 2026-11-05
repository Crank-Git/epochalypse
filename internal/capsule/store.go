package capsule

import (
	"io"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"

	bolt "go.etcd.io/bbolt"
)

// Store shares retained content transactions and keeps keys outside capsule records.
type Store struct {
	db             *installation.Handle
	owned          *installation.Store
	plaintextLimit int
}

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
	return Record{}, ErrNotFound
}

// Get returns an owner record independently of publication.
func (s *Store) Get(id string) (Record, error) { return Record{}, ErrNotFound }

// Published returns an object only after owner publication.
func (s *Store) Published(id string) (Record, error) { return Record{}, ErrNotFound }

// Publish commits publication after the stored key authenticates the object.
func (s *Store) Publish(id string, now time.Time) (Record, error) { return Record{}, ErrNotFound }

// Key returns a published key after the selected release condition permits it.
func (s *Store) Key(id string, now time.Time) ([32]byte, error) { return [32]byte{}, ErrNotFound }
