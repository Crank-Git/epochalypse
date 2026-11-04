package owner

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestSessionStoresOnlyTheIdentifierHash(t *testing.T) {
	s := sessionStore(t)
	token, err := s.SignIn([]byte("synthetic"), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if err := s.db.View(func(tx *bolt.Tx) error {
		key, data := tx.Bucket(sessionBucket).Cursor().First()
		if !bytes.Equal(key, digest[:]) {
			t.Fatal("session key is not the identifier hash")
		}
		if bytes.Contains(data, []byte(token)) || bytes.Contains(data, raw) {
			t.Fatal("session state contains the identifier")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionDoesNotReviveAfterExpiry(t *testing.T) {
	s := sessionStore(t)
	now := time.Now().UTC()
	token, err := s.SignIn([]byte("synthetic"), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(token, now.Add(30*time.Minute)); !errors.Is(err, ErrCredentials) {
		t.Fatalf("expired session: %v", err)
	}
	if _, err := s.Authenticate(token, now.Add(time.Minute)); !errors.Is(err, ErrCredentials) {
		t.Fatalf("revived session: %v", err)
	}
}

func TestSessionExpiresAtIdleBoundary(t *testing.T) {
	for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		t.Run(delta.String(), func(t *testing.T) {
			s := sessionStore(t)
			now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
			token, err := s.SignIn([]byte("synthetic"), now)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Authenticate(token, now.Add(30*time.Minute+delta))
			if delta < 0 && err != nil {
				t.Fatal(err)
			}
			if delta >= 0 && !errors.Is(err, ErrCredentials) {
				t.Fatalf("expired session: %v", err)
			}
		})
	}
}

func TestSessionExpiresAtAbsoluteBoundary(t *testing.T) {
	for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		t.Run(delta.String(), func(t *testing.T) {
			s := sessionStore(t)
			now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
			token, err := s.SignIn([]byte("synthetic"), now)
			if err != nil {
				t.Fatal(err)
			}
			for minute := 20; minute < 720; minute += 20 {
				if _, err := s.Authenticate(token, now.Add(time.Duration(minute)*time.Minute)); err != nil {
					t.Fatal(err)
				}
			}
			_, err = s.Authenticate(token, now.Add(12*time.Hour+delta))
			if delta < 0 && err != nil {
				t.Fatal(err)
			}
			if delta >= 0 && !errors.Is(err, ErrCredentials) {
				t.Fatalf("expired session: %v", err)
			}
		})
	}
}

func TestSessionSurvivesRestartAndRecoveryRevokesIt(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize([]byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	token, err := s.SignIn([]byte("synthetic"), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	if id, err := s.Authenticate(token, now.Add(time.Minute)); err != nil || len(id) != 32 {
		t.Fatalf("restarted session: %q %v", id, err)
	}
	if err := s.Recover([]byte("replacement")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(token, now.Add(2*time.Minute)); !errors.Is(err, ErrCredentials) {
		t.Fatalf("recovered session: %v", err)
	}
}

func TestSessionRejectsInvalidCredentialsAndClockRollback(t *testing.T) {
	s := sessionStore(t)
	now := time.Now().UTC()
	if _, err := s.SignIn([]byte("wrong"), now); !errors.Is(err, ErrCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	token, err := s.SignIn([]byte("synthetic"), now)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "invalid", token + "x"} {
		if _, err := s.Authenticate(value, now); !errors.Is(err, ErrCredentials) {
			t.Fatalf("invalid token: %v", err)
		}
	}
	if _, err := s.Authenticate(token, now.Add(-time.Nanosecond)); !errors.Is(err, ErrCredentials) {
		t.Fatalf("clock rollback: %v", err)
	}
	if err := s.SignOut(token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(token, now); !errors.Is(err, ErrCredentials) {
		t.Fatalf("signed-out session: %v", err)
	}
}

func sessionStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := s.Initialize([]byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	return s
}
