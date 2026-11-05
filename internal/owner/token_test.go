package owner

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

var tokenClock = time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)

func createTestToken(t *testing.T, s *Store, scopes ...string) TokenGrant {
	t.Helper()
	grant, err := s.CreateToken(TokenRequest{"automation", scopes, tokenClock.Add(time.Hour)}, tokenClock)
	if err != nil {
		t.Fatal(err)
	}
	return grant
}

func TestTokenAcceptsOnlyItsExactScopes(t *testing.T) {
	s := sessionStore(t)
	scopes := []string{"read", "capsule:write", "switch:write", "switch:checkin", "countdown:write"}
	for _, allowed := range scopes {
		grant := createTestToken(t, s, allowed)
		for _, scope := range scopes {
			id, err := s.AuthenticateToken(grant.Token, scope, tokenClock)
			if scope == allowed && (err != nil || len(id) != 32) {
				t.Fatalf("scope %s: %v", scope, err)
			}
			if scope != allowed && !errors.Is(err, ErrScope) {
				t.Fatalf("scope %s permits %s: %v", allowed, scope, err)
			}
		}
	}
	grant := createTestToken(t, s, scopes...)
	for _, scope := range scopes {
		if _, err := s.AuthenticateToken(grant.Token, scope, tokenClock); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AuthenticateToken(grant.Token, "admin", tokenClock); !errors.Is(err, ErrScope) {
		t.Fatal(err)
	}
}

func TestTokenExpiryHasNoIdleExtension(t *testing.T) {
	s := sessionStore(t)
	grant := createTestToken(t, s, "read")
	for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		_, err := s.AuthenticateToken(grant.Token, "read", tokenClock.Add(time.Hour+delta))
		if delta < 0 && err != nil {
			t.Fatal(err)
		}
		if delta >= 0 && !errors.Is(err, ErrCredentials) {
			t.Fatalf("expiry at %s: %v", delta, err)
		}
	}
	if _, err := s.AuthenticateToken(grant.Token, "read", tokenClock.Add(-time.Nanosecond)); !errors.Is(err, ErrCredentials) {
		t.Fatal("clock rollback permits token")
	}
}

func TestTokenStoresOnlyTheDigest(t *testing.T) {
	s := sessionStore(t)
	grant := createTestToken(t, s, "read")
	digest, err := sessionKey(grant.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		key, data := tx.Bucket(tokenBucket).Cursor().First()
		if !bytes.Equal(key, digest) {
			t.Fatal("token key differs from digest")
		}
		if bytes.Contains(data, []byte(grant.Token)) {
			t.Fatal("state contains token value")
		}
		var record tokenRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		if record.Digest != hex.EncodeToString(digest) || record.ID != grant.ID {
			t.Fatal("stored token metadata differs")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTokenRevocationSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize([]byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	grant := createTestToken(t, s, "read")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateToken(grant.Token, "read", tokenClock); err != nil {
		t.Fatal(err)
	}
	// Password recovery revokes sessions only.
	if err := s.Recover([]byte("replacement")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateToken(grant.Token, "read", tokenClock); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(grant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateToken(grant.Token, "read", tokenClock); !errors.Is(err, ErrCredentials) {
		t.Fatal("revocation requires restart")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.AuthenticateToken(grant.Token, "read", tokenClock); !errors.Is(err, ErrCredentials) {
		t.Fatal("restart restores token")
	}
	if err := s.RevokeToken(grant.ID); err != nil {
		t.Fatal(err)
	}
}

func TestTokenRejectsInvalidRequestsAndCredentials(t *testing.T) {
	s := sessionStore(t)
	for _, request := range []TokenRequest{
		{"", []string{"read"}, tokenClock.Add(time.Hour)},
		{strings.Repeat("x", 129), []string{"read"}, tokenClock.Add(time.Hour)},
		{"\xff", []string{"read"}, tokenClock.Add(time.Hour)},
		{"automation", nil, tokenClock.Add(time.Hour)},
		{"automation", []string{"admin"}, tokenClock.Add(time.Hour)},
		{"automation", []string{"read", "read"}, tokenClock.Add(time.Hour)},
		{"automation", []string{"read", "read", "read", "read", "read", "read"}, tokenClock.Add(time.Hour)},
		{"automation", []string{"read"}, tokenClock},
	} {
		if _, err := s.CreateToken(request, tokenClock); !errors.Is(err, ErrTokenRequest) {
			t.Fatalf("invalid request: %v", err)
		}
	}
	for _, value := range []string{"", "bad", strings.Repeat("!", 43), strings.Repeat("A", 43)} {
		if _, err := s.AuthenticateToken(value, "read", tokenClock); !errors.Is(err, ErrCredentials) {
			t.Fatalf("invalid credential: %v", err)
		}
	}
	for _, id := range []string{"bad", strings.Repeat("z", 32), strings.Repeat("A", 32)} {
		if err := s.RevokeToken(id); !errors.Is(err, ErrTokenRequest) {
			t.Fatal("invalid id accepted")
		}
	}
}

func TestTokenPreservesTheVersionOneFixture(t *testing.T) {
	s := sessionStore(t)
	data, err := os.ReadFile("testdata/token-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var record tokenRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		identity, err := readIdentity(tx)
		if err != nil {
			return err
		}
		record.OwnerID = identity.ID
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		digest, err := hex.DecodeString(record.Digest)
		if err != nil {
			return err
		}
		return tx.Bucket(tokenBucket).Put(digest, encoded)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateToken(strings.Repeat("A", 43), "read", tokenClock); err != nil {
		t.Fatal(err)
	}
}

func TestTokenRejectsInvalidStateAndClosedStore(t *testing.T) {
	s := sessionStore(t)
	grant := createTestToken(t, s, "read")
	digest, err := sessionKey(grant.Token)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []string{"bad", `{"version":2}`, `{"version":1}`} {
		if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(tokenBucket).Put(digest, []byte(fixture)) }); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AuthenticateToken(grant.Token, "read", tokenClock); err == nil {
			t.Fatal("invalid token state authenticates")
		}
	}
	if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(tokenBucket).Put(digest, []byte("bad")) }); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(grant.ID); err == nil {
		t.Fatal("invalid token state revokes")
	}
	// The helper owns the final close.
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateToken(grant.Token, "read", tokenClock); err == nil {
		t.Fatal("closed store authenticates")
	}
	if _, err := s.CreateToken(TokenRequest{"automation", []string{"read"}, tokenClock.Add(time.Hour)}, tokenClock); err == nil {
		t.Fatal("closed store creates token")
	}
	if err := s.RevokeToken(grant.ID); err == nil {
		t.Fatal("closed store revokes token")
	}
}
