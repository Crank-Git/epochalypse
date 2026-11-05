package installation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Store owns one database handle for retained installation content.
type Store struct {
	db *bolt.DB
	// Limits remain fixed after the caller starts concurrent operations.
	Limits Limits
}

// Open preserves the version 1 database path and validates the retention limits.
func Open(stateDir string, limits Limits) (*Store, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	directory := filepath.Join(stateDir, "v1")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create installation state directory: %w", err)
	}
	if err := os.Chmod(directory, 0700); err != nil { // #nosec G302 -- Retained content requires owner-only access.
		return nil, fmt.Errorf("protect installation state directory: %w", err)
	}
	path := filepath.Join(directory, "windows.db")
	info, err := os.Lstat(path)
	if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return nil, errors.New("installation database requires an owner-only regular file")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect installation database: %w", err)
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open installation database: %w", err)
	}
	return &Store{db: db, Limits: limits}, nil
}

// Update commits one atomic content transaction.
func (s *Store) Update(fn func(*bolt.Tx) error) error { return s.db.Update(fn) }

// View reads one consistent content transaction.
func (s *Store) View(fn func(*bolt.Tx) error) error { return s.db.View(fn) }

// Close releases the installation database lock after all domain users stop.
func (s *Store) Close() error { return s.db.Close() }
