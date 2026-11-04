package owner

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

var auditBucket = []byte("audit-v1")

const auditCapacity = 256

type auditAction uint8

const (
	ownerInitialized auditAction = iota
	ownerRecovered
	signInRejected
	sessionCreated
	sessionRevoked
	tokenCreated
	tokenRevoked
)

var auditNames = [...]string{"owner_initialized", "owner_recovered", "sign_in_rejected", "session_created", "session_revoked", "token_created", "token_revoked"}

// AuditEvent contains only a fixed action and a trusted server time.
// Credential values and caller-supplied text have no field in this format.
type AuditEvent struct {
	Version int       `json:"version"`
	At      time.Time `json:"at"`
	Action  string    `json:"action"`
}

func appendAudit(tx *bolt.Tx, action auditAction, now time.Time) error {
	if int(action) >= len(auditNames) || now.IsZero() {
		return errors.New("invalid audit event")
	}
	data, err := json.Marshal(AuditEvent{1, now.UTC(), auditNames[action]})
	if err != nil {
		return fmt.Errorf("encode audit event: %w", err)
	}
	bucket := tx.Bucket(auditBucket)
	if bucket == nil {
		return errors.New("audit state is unavailable")
	}
	sequence, err := bucket.NextSequence()
	if err != nil {
		return fmt.Errorf("sequence audit event: %w", err)
	}
	if err := bucket.Put(binary.BigEndian.AppendUint64(nil, sequence), data); err != nil {
		return fmt.Errorf("persist audit event: %w", err)
	}
	if sequence > auditCapacity {
		return bucket.Delete(binary.BigEndian.AppendUint64(nil, sequence-auditCapacity))
	}
	return nil
}

// AuditEvents returns the newest events first, with a limit from 1 through 100.
// The caller must authorize owner access. Invalid limits or durable state return an error.
func (s *Store) AuditEvents(limit int) ([]AuditEvent, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("audit limit must be from 1 through 100")
	}
	events := make([]AuditEvent, 0, limit)
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(auditBucket)
		if bucket == nil {
			return errors.New("audit state is unavailable")
		}
		cursor := bucket.Cursor()
		for key, value := cursor.Last(); key != nil && len(events) < limit; key, value = cursor.Prev() {
			var event AuditEvent
			if err := json.Unmarshal(value, &event); err != nil || !event.valid() {
				return errors.New("invalid audit state")
			}
			events = append(events, event)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read audit events: %w", err)
	}
	return events, nil
}

func (event AuditEvent) valid() bool {
	if event.Version != 1 || event.At.IsZero() {
		return false
	}
	for _, name := range auditNames {
		if event.Action == name {
			return true
		}
	}
	return false
}
