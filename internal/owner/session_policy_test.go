package owner

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestRejectedCSRFDoesNotRenewTheOwnerSession(t *testing.T) {
	for _, proof := range []string{"missing", "wrong", "foreign-origin", "sibling-origin"} {
		t.Run(proof, func(t *testing.T) {
			s := sessionStore(t)
			created := time.Now().UTC().Add(-29 * time.Minute)
			token, err := s.SignIn([]byte("synthetic"), created)
			if err != nil {
				t.Fatal(err)
			}
			s.webNow = func() time.Time { return created.Add(29 * time.Minute) }
			r := httptest.NewRequest("POST", "https://localhost/api/v1/tokens", nil)
			r.Header.Set("Cookie", cookieName+"="+token)
			if proof != "missing" {
				addCSRF(t, s, r)
			}
			switch proof {
			case "wrong":
				r.Header.Set("X-CSRF-Token", "wrong")
			case "foreign-origin":
				r.Header.Set("Origin", "https://foreign.example")
			case "sibling-origin":
				r.Header.Set("Origin", "https://sibling.localhost")
			}
			w := httptest.NewRecorder()
			s.Handler(false).ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("rejected proof returns %d", w.Code)
			}
			if seen := sessionSeen(t, s, token); !seen.Equal(created) {
				t.Fatalf("rejected CSRF renews Seen from %s to %s", created, seen)
			}
			if _, err := s.Authenticate(token, created.Add(30*time.Minute)); !errors.Is(err, ErrCredentials) {
				t.Fatalf("rejected CSRF extends idle expiry: %v", err)
			}
		})
	}
}

func TestAcceptedOwnerRequestsRenewTheSession(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		t.Run(method, func(t *testing.T) {
			s := sessionStore(t)
			created := time.Now().UTC().Add(-29 * time.Minute)
			token, err := s.SignIn([]byte("synthetic"), created)
			if err != nil {
				t.Fatal(err)
			}
			now := created.Add(29 * time.Minute)
			s.webNow = func() time.Time { return now }
			r := httptest.NewRequest(method, "https://localhost/protected", nil)
			r.Header.Set("Cookie", cookieName+"="+token)
			if method == "POST" {
				addCSRF(t, s, r)
			}
			w := httptest.NewRecorder()
			s.RequireOwner(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
			if w.Code != 204 {
				t.Fatalf("valid request returns %d", w.Code)
			}
			if seen := sessionSeen(t, s, token); !seen.Equal(now) {
				t.Fatalf("accepted request does not renew Seen: %s", seen)
			}
			if _, err := s.Authenticate(token, created.Add(30*time.Minute)); err != nil {
				t.Fatalf("accepted request does not extend idle expiry: %v", err)
			}
		})
	}
}

func sessionSeen(t *testing.T, s *Store, token string) time.Time {
	t.Helper()
	key, err := sessionKey(token)
	if err != nil {
		t.Fatal(err)
	}
	var record session
	if err := s.db.View(func(tx *bolt.Tx) error { return json.Unmarshal(tx.Bucket(sessionBucket).Get(key), &record) }); err != nil {
		t.Fatal(err)
	}
	return record.Seen
}

func TestOwnerGuardRechecksRevocationAfterCSRF(t *testing.T) {
	s := sessionStore(t)
	token, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "https://localhost/owner/sign-out", nil)
	r.Header.Set("Cookie", cookieName+"="+token)
	addCSRF(t, s, r)
	proof := r.Header.Get("X-CSRF-Token")
	r.Header.Del("X-CSRF-Token")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body := "csrf_token=" + url.QueryEscape(proof)
	r.Body = io.NopCloser(&policyReader{Reader: strings.NewReader(body), before: func() {
		if err := s.SignOut(token); err != nil {
			t.Fatal(err)
		}
	}})
	w := httptest.NewRecorder()
	s.RequireOwner(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("revoked session reaches mutation: %d %s", w.Code, w.Body)
	}
}

type policyReader struct {
	io.Reader
	before func()
}

func (r *policyReader) Read(data []byte) (int, error) {
	if r.before != nil {
		r.before()
		r.before = nil
	}
	return r.Reader.Read(data)
}
