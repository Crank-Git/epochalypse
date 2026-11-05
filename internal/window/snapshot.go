package window

import (
	"errors"

	bolt "go.etcd.io/bbolt"
)

// ValidateSnapshot verifies every retained window and observation in one read transaction.
func (s *Store) validateSnapshot(tx *bolt.Tx) error {
	if s == nil || tx == nil {
		return errors.New("window snapshot validator requires a store and transaction")
	}
	config := tx.Bucket(configurationBucket)
	if config == nil || string(config.Get([]byte("dnskey"))) != s.signer.key.String() {
		return errors.New("snapshot window configuration is unsupported")
	}
	if err := config.ForEach(func(key, value []byte) error {
		if string(key) != "dnskey" || value == nil {
			return errors.New("snapshot window configuration is unsupported")
		}
		return nil
	}); err != nil {
		return err
	}
	windows := tx.Bucket(windowBucket)
	if windows != nil {
		if err := windows.ForEach(func(id, data []byte) error {
			w, err := s.decode(data)
			if err != nil || string(id) != w.ID {
				return errors.New("snapshot contains an invalid window")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	observations := tx.Bucket(observationBucket)
	if observations == nil {
		return nil
	}
	return observations.ForEach(func(key, data []byte) error {
		if _, err := s.runAt(tx, key, data); err != nil {
			return errors.New("snapshot contains an invalid observation")
		}
		return nil
	})
}

// ValidateSnapshot verifies window state without opening or changing the database.
func ValidateSnapshot(tx *bolt.Tx, signer *Signer, limits Limits) error {
	if signer == nil || signer.key == nil || signer.private == nil {
		return errors.New("configure the window signer before snapshot validation")
	}
	if err := limits.Validate(); err != nil {
		return err
	}
	return (&Store{signer: signer, limits: &limits}).validateSnapshot(tx)
}
