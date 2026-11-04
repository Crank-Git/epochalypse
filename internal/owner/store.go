package owner

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

var (
	ErrExists      = errors.New("owner already exists")
	ErrCredentials = errors.New("invalid credentials")
	stateBucket    = []byte("owner-v1")
	identityKey    = []byte("identity")
	sessionBucket  = []byte("sessions-v1")
)

// Identity contains the single local owner record.
type Identity struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Password  Password  `json:"password"`
}

// Store owns the synced local database.
type Store struct{ db *bolt.DB }

// Open opens version 1 owner state with owner-only filesystem access.
func Open(stateDir string) (*Store, error) {
	directory := filepath.Join(stateDir, "v1")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create owner state: %w", err)
	}
	// Directory traversal requires the owner execute bit.
	if err := os.Chmod(directory, 0700); err != nil { // #nosec G302
		return nil, fmt.Errorf("protect owner directory: %w", err)
	}
	path := filepath.Join(directory, "owner.db")
	info, err := os.Lstat(path)
	if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return nil, errors.New("owner database requires an owner-only regular file")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect owner database: %w", err)
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open owner database: %w", err)
	}
	s := &Store{db: db}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{stateBucket, sessionBucket} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize owner database: %w", err)
	}
	return s, nil
}

// Close releases the database file lock.
func (s *Store) Close() error { return s.db.Close() }

func readIdentity(tx *bolt.Tx) (Identity, error) {
	var identity Identity
	data := tx.Bucket(stateBucket).Get(identityKey)
	if data == nil {
		return identity, ErrCredentials
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		return identity, errors.New("invalid owner state")
	}
	if identity.Version != 1 || len(identity.ID) != 32 || identity.CreatedAt.IsZero() {
		return identity, errors.New("unsupported owner state")
	}
	if _, _, err := identity.Password.validate(); err != nil {
		return identity, err
	}
	return identity, nil
}

// Initialize creates one owner and refuses a replacement.
func (s *Store) Initialize(password []byte) error { return s.setPassword(password, false) }

// Recover replaces the local password and revokes all sessions atomically.
func (s *Store) Recover(password []byte) error { return s.setPassword(password, true) }

func (s *Store) setPassword(password []byte, recoverOwner bool) error {
	hash, err := newPassword(password)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		identity, err := readIdentity(tx)
		if recoverOwner && err != nil {
			return err
		}
		if !recoverOwner {
			if tx.Bucket(stateBucket).Get(identityKey) != nil {
				return ErrExists
			}
			id := make([]byte, 16)
			_, _ = rand.Read(id)
			identity = Identity{Version: 1, ID: hex.EncodeToString(id), CreatedAt: time.Now().UTC()}
		}
		identity.Password = hash
		encoded, err := json.Marshal(identity)
		if err != nil {
			return err
		}
		if err := tx.Bucket(stateBucket).Put(identityKey, encoded); err != nil {
			return err
		}
		if err := tx.DeleteBucket(sessionBucket); err != nil {
			return err
		}
		_, err = tx.CreateBucket(sessionBucket)
		return err
	})
}

// Verify accepts only the current owner's password.
func (s *Store) Verify(password []byte) error {
	var identity Identity
	err := s.db.View(func(tx *bolt.Tx) error { var err error; identity, err = readIdentity(tx); return err })
	if err != nil {
		return err
	}
	if !identity.Password.matches(password) {
		return ErrCredentials
	}
	return nil
}
