package window

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

// ErrNotFound identifies a missing public window.
var ErrNotFound = errors.New("window does not exist")

// Store owns the durable public window state.
type Store struct {
	db     *installation.Handle
	signer *Signer
	limits *Limits
	owned  *installation.Store
}

var windowBucket = []byte(installation.WindowsBucket)
var configurationBucket = []byte(installation.ConfigurationBucket)

// Open opens version 1 window state independently of owner credentials.
func Open(stateDir string, signer *Signer) (*Store, error) {
	return OpenWithLimits(stateDir, signer, DefaultLimits())
}

func openStore(stateDir string, signer *Signer, limits Limits) (*Store, error) {
	if signer == nil || signer.key == nil || signer.private == nil {
		return nil, errors.New("configure the window signer before state access")
	}
	db, err := installation.Open(stateDir, limits)
	if err != nil {
		return nil, err
	}
	store, err := Attach(db, signer)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	store.owned = db
	return store, nil
}

// Attach shares the installation handle without ownership of its lifetime.
// The caller closes the installation after all attached domains stop.
func Attach(db *installation.Store, signer *Signer) (*Store, error) {
	if db == nil || signer == nil || signer.key == nil || signer.private == nil {
		return nil, errors.New("configure installation state and the window signer before state access")
	}
	store := &Store{db: db.Handle(), signer: signer, limits: &db.Limits}
	if err := store.db.Update(store.initialize); err != nil {
		return nil, fmt.Errorf("initialize window database: %w", err)
	}
	return store, nil
}

func (s *Store) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(windowBucket); err != nil {
		return err
	}
	if _, err := tx.CreateBucketIfNotExists(observationBucket); err != nil {
		return err
	}
	config, err := tx.CreateBucketIfNotExists(configurationBucket)
	if err != nil {
		return err
	}
	key := []byte(s.signer.key.String())
	if previous := config.Get([]byte("dnskey")); previous != nil && string(previous) != string(key) {
		return errors.New("window state requires its original zone and DNSKEY")
	}
	return config.Put([]byte("dnskey"), key)
}

// Close stops window access and releases the database only when Open owns it.
func (s *Store) Close() error {
	_ = s.db.Close()
	if s.owned != nil {
		return s.owned.Close()
	}
	return nil
}

// Create persists a signed window before it returns the public record.
func (s *Store) Create(in Input, now time.Time) (Window, error) {
	var random [16]byte
	_, _ = rand.Read(random[:])
	w, err := s.signer.create(hex.EncodeToString(random[:]), in, now)
	if err != nil {
		return Window{}, err
	}
	encoded, err := json.Marshal(w)
	if err != nil {
		return Window{}, fmt.Errorf("encode public window: %w", err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(windowBucket)
		if err := s.admits(tx, len(encoded)); err != nil {
			return err
		}
		if bucket.Get([]byte(w.ID)) != nil {
			return errors.New("window identifier already exists")
		}
		return bucket.Put([]byte(w.ID), encoded)
	}); err != nil {
		return Window{}, fmt.Errorf("persist public window: %w", err)
	}
	return w, nil
}

// Get returns one immutable public window.
func (s *Store) Get(id string) (Window, error) {
	if !validID(id) {
		return Window{}, errors.New("invalid window identifier")
	}
	var w Window
	err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		w, err = s.decode(tx.Bucket(windowBucket).Get([]byte(id)))
		return err
	})
	return w, err
}

// List returns the immutable public windows in identifier order.
func (s *Store) List() ([]Window, error) {
	windows, _, err := s.Snapshot()
	return windows, err
}

// Revision returns the current transaction identifier for an ephemeral read cache.
func (s *Store) Revision() (int, error) {
	var revision int
	err := s.db.View(func(tx *bolt.Tx) error { revision = tx.ID(); return nil })
	return revision, err
}

// Snapshot returns verified windows and one consistent cache revision.
func (s *Store) Snapshot() ([]Window, int, error) {
	windows := []Window{}
	var revision int
	err := s.db.View(func(tx *bolt.Tx) error {
		revision = tx.ID()
		return tx.Bucket(windowBucket).ForEach(func(_, data []byte) error {
			w, err := s.decode(data)
			if err != nil {
				return err
			}
			windows = append(windows, w)
			return nil
		})
	})
	return windows, revision, err
}

func (s *Store) decode(data []byte) (Window, error) {
	if data == nil {
		return Window{}, ErrNotFound
	}
	var w Window
	if err := json.Unmarshal(data, &w); err != nil {
		return Window{}, errors.New("invalid durable window document")
	}
	if w.DNSKEY != s.signer.key.String() {
		return Window{}, errors.New("durable window differs from the configured DNSKEY")
	}
	if _, _, _, err := w.Records(); err != nil {
		return Window{}, fmt.Errorf("verify durable window: %w", err)
	}
	return w, nil
}
