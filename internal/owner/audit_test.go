package owner

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestOwnerOperationsRecordOnlySafeAuditFields(t *testing.T) {
	s := sessionStore(t)
	now := time.Now().UTC()
	if _, err := s.SignIn([]byte("private-password"), now); err != ErrCredentials {
		t.Fatal(err)
	}
	session, err := s.SignIn([]byte("synthetic"), now)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.CreateToken(TokenRequest{Name: "private-name-secret-key-share", Scopes: []string{"read"}, ExpiresAt: now.Add(time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(grant.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SignOut(session); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover([]byte("replacement-password")); err != nil {
		t.Fatal(err)
	}
	want := []string{"owner_initialized", "sign_in_rejected", "session_created", "token_created", "token_revoked", "session_revoked", "owner_recovered"}
	index := 0
	if err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("audit-v1"))
		if bucket == nil {
			t.Fatal("owner operations do not create an audit log")
		}
		return bucket.ForEach(func(_, value []byte) error {
			action := assertSafeAuditFields(t, value, session, grant.Token)
			if index >= len(want) || action != want[index] {
				t.Fatalf("event %d: %s", index, action)
			}
			index++
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if index != len(want) {
		t.Fatal("audit log omits an operation")
	}
}

func assertSafeAuditFields(t *testing.T, value []byte, session, token string) string {
	t.Helper()
	for _, secret := range []string{"synthetic", "private-password", "private-name-secret-key-share", session, token, "replacement-password"} {
		if bytes.Contains(value, []byte(secret)) {
			t.Fatal("audit event contains a secret")
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 || fields["version"] == nil || fields["at"] == nil || fields["action"] == nil {
		t.Fatalf("unsafe fields: %s", value)
	}
	var action string
	if err := json.Unmarshal(fields["action"], &action); err != nil {
		t.Fatal(err)
	}
	return action
}

func TestAuditRetainsABoundedHistoryAcrossRestart(t *testing.T) {
	directory := t.TempDir()
	s, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	if err := s.db.Update(func(tx *bolt.Tx) error {
		for i := range auditCapacity + 4 {
			if err := appendAudit(tx, signInRejected, now.Add(time.Duration(i)*time.Second)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	events, err := s.AuditEvents(100)
	if err != nil || len(events) != 100 {
		t.Fatalf("bounded read: %d %v", len(events), err)
	}
	if !events[0].At.Equal(now.Add(259*time.Second)) || !events[99].At.Equal(now.Add(160*time.Second)) {
		t.Fatal("events do not use newest-first order")
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		if count := tx.Bucket(auditBucket).Stats().KeyN; count != auditCapacity {
			t.Fatalf("retained events: %d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{-1, 0, 101} {
		if _, err := s.AuditEvents(limit); err == nil {
			t.Fatal("invalid limit succeeds")
		}
	}
}

func TestAuditRejectsInvalidState(t *testing.T) {
	s := sessionStore(t)
	for _, fixture := range []string{`bad`, `{"version":2,"at":"2026-11-04T00:00:00Z","action":"session_created"}`, `{"version":1,"action":"session_created"}`, `{"version":1,"at":"2026-11-04T00:00:00Z","action":"secret"}`} {
		if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(auditBucket).Put([]byte("z"), []byte(fixture)) }); err != nil {
			t.Fatal(err)
		}
		if events, err := s.AuditEvents(1); err == nil || events != nil {
			t.Fatal("invalid event returns audit data")
		}
	}
	for _, event := range []struct {
		action auditAction
		at     time.Time
	}{{255, time.Now()}, {sessionCreated, time.Time{}}, {sessionCreated, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}} {
		if err := s.db.Update(func(tx *bolt.Tx) error { return appendAudit(tx, event.action, event.at) }); err == nil {
			t.Fatal("invalid event succeeds")
		}
	}
	if err := s.db.View(func(tx *bolt.Tx) error { return appendAudit(tx, sessionCreated, time.Now()) }); err == nil {
		t.Fatal("read-only event succeeds")
	}
	if err := s.db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket(auditBucket) }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuditEvents(1); err == nil {
		t.Fatal("missing audit state succeeds")
	}
}

func TestAuditFailureRollsBackOwnerMutations(t *testing.T) {
	s := sessionStore(t)
	now := time.Now()
	session, err := s.SignIn([]byte("synthetic"), now)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.CreateToken(TokenRequest{Name: "test", Scopes: []string{"read"}, ExpiresAt: now.Add(time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket(auditBucket) }); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover([]byte("replacement")); err == nil {
		t.Fatal("recovery succeeds without audit state")
	}
	if _, err := s.SignIn([]byte("synthetic"), now); err == nil {
		t.Fatal("sign-in succeeds without audit state")
	}
	if _, err := s.SignIn([]byte("wrong"), now); err == nil || errors.Is(err, ErrCredentials) {
		t.Fatal("audit failure is hidden")
	}
	if _, err := s.CreateToken(TokenRequest{Name: "test", Scopes: []string{"read"}, ExpiresAt: now.Add(time.Hour)}, now); err == nil {
		t.Fatal("token creation succeeds without audit state")
	}
	if err := s.SignOut(session); err == nil {
		t.Fatal("sign-out succeeds without audit state")
	}
	if err := s.RevokeToken(grant.ID); err == nil {
		t.Fatal("token revocation succeeds without audit state")
	}
	if err := s.Verify([]byte("synthetic")); err != nil {
		t.Fatal("failed recovery changes password")
	}
	if _, err := s.Authenticate(session, now); err != nil {
		t.Fatal("failed sign-out revokes session")
	}
	if _, err := s.AuthenticateToken(grant.Token, "read", now); err != nil {
		t.Fatal("failed revocation deletes token")
	}
}
