package window

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// ErrNotFound identifies a missing public window.
var ErrNotFound = errors.New("window does not exist")

// Store owns the durable public window state.
type Store struct {
	db     *bolt.DB
	signer *Signer
}

var windowBucket = []byte("windows-v1")
var configurationBucket = []byte("configuration-v1")

// Open opens version 1 window state independently of owner credentials.
func Open(stateDir string, signer *Signer) (*Store, error) {
	if signer == nil || signer.key == nil || signer.private == nil {
		return nil, errors.New("configure the window signer before state access")
	}
	directory := filepath.Join(stateDir, "v1")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create window state directory: %w", err)
	}
	// The owner and window stores share this versioned directory.
	if err := os.Chmod(directory, 0700); err != nil { // #nosec G302
		return nil, fmt.Errorf("protect window state directory: %w", err)
	}
	path := filepath.Join(directory, "windows.db")
	info, err := os.Lstat(path)
	if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return nil, errors.New("window database requires an owner-only regular file")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect window database: %w", err)
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open window database: %w", err)
	}
	store := &Store{db: db, signer: signer}
	if err := db.Update(store.initialize); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize window database: %w", err)
	}
	return store, nil
}

func (s *Store) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(windowBucket); err != nil {
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

// Close releases the window database lock.
func (s *Store) Close() error { return s.db.Close() }

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
	windows := []Window{}
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(windowBucket).ForEach(func(_, data []byte) error {
			w, err := s.decode(data)
			if err != nil {
				return err
			}
			windows = append(windows, w)
			return nil
		})
	})
	return windows, err
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
