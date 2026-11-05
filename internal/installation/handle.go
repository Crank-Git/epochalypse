package installation

import (
	"errors"
	"sync/atomic"

	bolt "go.etcd.io/bbolt"
)

// Handle permits one domain to stop without closing another domain's database.
type Handle struct {
	store  *Store
	closed atomic.Bool
}

// Handle returns a domain handle that shares the installation budget and database.
func (s *Store) Handle() *Handle { return &Handle{store: s} }

// View reads one transaction unless the domain has stopped.
func (h *Handle) View(fn func(*bolt.Tx) error) error {
	if h.closed.Load() {
		return errors.New("retained content handle is closed")
	}
	return h.store.View(fn)
}

// Update commits one transaction unless the domain has stopped.
func (h *Handle) Update(fn func(*bolt.Tx) error) error {
	if h.closed.Load() {
		return errors.New("retained content handle is closed")
	}
	return h.store.Update(fn)
}

// Admit checks the shared allowance inside the domain's write transaction.
func (h *Handle) Admit(tx *bolt.Tx, objects int, size int64) error {
	return h.store.Admit(tx, objects, size)
}

// Close stops new domain transactions and preserves the shared database lifetime.
func (h *Handle) Close() error {
	h.closed.Store(true)
	return nil
}
