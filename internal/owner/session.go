package owner

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

type session struct {
	Version int       `json:"version"`
	OwnerID string    `json:"owner_id"`
	Created time.Time `json:"created"`
	Seen    time.Time `json:"seen"`
}

// SignIn persists a session after the password matches the current owner.
// The caller supplies its trusted clock value.
func (s *Store) SignIn(password []byte, now time.Time) (string, error) {
	var identity Identity
	err := s.db.View(func(tx *bolt.Tx) error { var err error; identity, err = readIdentity(tx); return err })
	if err != nil {
		return "", err
	}
	if !identity.Password.matches(password) {
		if err := s.db.Update(func(tx *bolt.Tx) error { return appendAudit(tx, signInRejected, now) }); err != nil {
			return "", err
		}
		return "", ErrCredentials
	}
	return s.persistSession(identity, now)
}

func (s *Store) persistSession(identity Identity, now time.Time) (string, error) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	key := sha256.Sum256(raw)
	encoded, err := json.Marshal(session{1, identity.ID, now.UTC(), now.UTC()})
	if err != nil {
		return "", fmt.Errorf("encode session: %w", err)
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		current, err := readIdentity(tx)
		if err != nil {
			return err
		}
		// Recovery must not permit an in-flight password check to restore access.
		if current.Password != identity.Password {
			return ErrCredentials
		}
		if err := tx.Bucket(sessionBucket).Put(key[:], encoded); err != nil {
			return err
		}
		return appendAudit(tx, sessionCreated, now)
	})
	if err != nil {
		return "", fmt.Errorf("persist session: %w", err)
	}
	return token, nil
}

func sessionKey(token string) ([]byte, error) {
	if len(token) != 43 {
		return nil, ErrCredentials
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, ErrCredentials
	}
	key := sha256.Sum256(raw)
	return key[:], nil
}

// Authenticate returns the owner ID and renews the idle deadline.
// The caller supplies its trusted clock value. Expired sessions return ErrCredentials.
func (s *Store) Authenticate(token string, now time.Time) (string, error) {
	return s.authenticateSession(token, now, true)
}

func (s *Store) authenticateSession(token string, now time.Time, renew bool) (string, error) {
	key, err := sessionKey(token)
	if err != nil {
		return "", err
	}
	var record session
	rejected := false
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(sessionBucket)
		data := bucket.Get(key)
		if data == nil {
			return ErrCredentials
		}
		if err := json.Unmarshal(data, &record); err != nil {
			return errors.New("invalid session state")
		}
		if !record.valid(now) {
			// Commit deletion so a later clock rollback cannot revive this session.
			rejected = true
			return bucket.Delete(key)
		}
		identity, err := readIdentity(tx)
		if err != nil {
			return err
		}
		if record.OwnerID != identity.ID {
			return ErrCredentials
		}
		if !renew {
			return nil
		}
		record.Seen = now.UTC()
		encoded, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("encode session: %w", err)
		}
		return bucket.Put(key, encoded)
	})
	if err != nil {
		return "", err
	}
	if rejected {
		return "", ErrCredentials
	}
	return record.OwnerID, nil
}

func (r session) valid(now time.Time) bool {
	return r.Version == 1 && !r.Created.IsZero() && !r.Seen.Before(r.Created) &&
		!now.Before(r.Seen) && now.Before(r.Seen.Add(30*time.Minute)) && now.Before(r.Created.Add(12*time.Hour))
}

// SignOut revokes a session. Repeated calls succeed.
func (s *Store) SignOut(token string) error {
	key, err := sessionKey(token)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(sessionBucket)
		if bucket.Get(key) == nil {
			return nil
		}
		if err := bucket.Delete(key); err != nil {
			return err
		}
		return appendAudit(tx, sessionRevoked, time.Now())
	})
}
