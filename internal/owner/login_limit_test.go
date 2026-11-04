package owner

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoginCooldownUsesTheFifthFailureTime(t *testing.T) {
	for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		t.Run(delta.String(), func(t *testing.T) {
			var limiter loginLimiter
			now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
			clock := func() time.Time { return now }
			fail := func(time.Time) (string, error) { return "", ErrCredentials }
			for attempt := range 5 {
				if attempt == 4 {
					now = now.Add(14 * time.Minute)
				}
				if _, err := limiter.attempt("peer", clock, fail); !errors.Is(err, ErrCredentials) {
					t.Fatal(err)
				}
			}
			now = now.Add(loginWindow + delta)
			called := false
			_, err := limiter.attempt("peer", clock, func(time.Time) (string, error) { called = true; return "session", nil })
			if delta < 0 {
				if !errors.Is(err, errLoginLimited) || called {
					t.Fatal("cooldown ends early")
				}
			} else if err != nil || !called {
				t.Fatal("cooldown persists at its boundary")
			}
		})
	}
}

func TestLoginWindowResetsAtItsBoundary(t *testing.T) {
	for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		var limiter loginLimiter
		now := time.Now()
		clock := func() time.Time { return now }
		fail := func(time.Time) (string, error) { return "", ErrCredentials }
		for range 4 {
			_, _ = limiter.attempt("peer", clock, fail)
		}
		now = now.Add(loginWindow + delta)
		_, _ = limiter.attempt("peer", clock, fail)
		_, err := limiter.attempt("peer", clock, fail)
		if delta < 0 && !errors.Is(err, errLoginLimited) {
			t.Fatal("window resets early")
		}
		if delta >= 0 && !errors.Is(err, ErrCredentials) {
			t.Fatal("window persists at its boundary")
		}
	}
}

func TestLoginLimitsIdentityAcrossPeerSources(t *testing.T) {
	var limiter loginLimiter
	now := time.Now()
	for attempt := range 5 {
		_, err := limiter.attempt(fmt.Sprint(attempt), func() time.Time { return now }, func(time.Time) (string, error) { return "", ErrCredentials })
		if !errors.Is(err, ErrCredentials) {
			t.Fatal(err)
		}
	}
	_, err := limiter.attempt("new-peer", func() time.Time { return now }, func(time.Time) (string, error) { t.Fatal("identity cooldown permits a password check"); return "", nil })
	if !errors.Is(err, errLoginLimited) {
		t.Fatal("source rotation bypasses identity limit")
	}
}

func TestLoginRetainsFailuresAfterSuccessAndClockRollback(t *testing.T) {
	var limiter loginLimiter
	now := time.Now()
	clock := func() time.Time { return now }
	fail := func(time.Time) (string, error) { return "", ErrCredentials }
	for range 4 {
		_, _ = limiter.attempt("peer", clock, fail)
	}
	if _, err := limiter.attempt("peer", clock, func(time.Time) (string, error) { return "session", nil }); err != nil {
		t.Fatal(err)
	}
	now = now.Add(-time.Hour)
	_, _ = limiter.attempt("peer", clock, fail)
	_, err := limiter.attempt("peer", clock, fail)
	if !errors.Is(err, errLoginLimited) {
		t.Fatal("success or clock rollback removes failures")
	}
	now = now.Add(time.Hour)
	if _, err := limiter.attempt("peer", clock, fail); !errors.Is(err, errLoginLimited) {
		t.Fatal("clock correction ends the cooldown early")
	}
}

func TestLoginSourceStateHasABoundAndExpires(t *testing.T) {
	now := time.Now()
	limiter := loginLimiter{sources: make(map[string]loginFailures)}
	for source := range maxLoginSources {
		limiter.sources[fmt.Sprint(source)] = loginFailures{count: 1, start: now}
	}
	clock := func() time.Time { return now }
	fail := func(time.Time) (string, error) { return "", ErrCredentials }
	if _, err := limiter.attempt("new-peer", clock, fail); !errors.Is(err, errLoginLimited) || len(limiter.sources) != maxLoginSources {
		t.Fatal("source map exceeds its bound")
	}
	if _, err := limiter.attempt("0", clock, fail); !errors.Is(err, ErrCredentials) {
		t.Fatal("existing source fails at capacity")
	}
	now = now.Add(loginWindow)
	if _, err := limiter.attempt("new-peer", clock, fail); !errors.Is(err, ErrCredentials) || len(limiter.sources) != 1 {
		t.Fatal("source map retains expired failures")
	}
}

func TestLoginSourceCooldownRejectsWithoutIdentityState(t *testing.T) {
	now := time.Now()
	limiter := loginLimiter{sources: map[string]loginFailures{"peer": {count: 5, start: now, until: now.Add(loginWindow)}}}
	_, err := limiter.attempt("peer", func() time.Time { return now }, func(time.Time) (string, error) { t.Fatal("source cooldown permits a password check"); return "", nil })
	if !errors.Is(err, errLoginLimited) {
		t.Fatal("source cooldown fails")
	}
}

func TestLoginServiceErrorsDoNotConsumeFailures(t *testing.T) {
	var limiter loginLimiter
	for range 6 {
		_, err := limiter.attempt("peer", time.Now, func(time.Time) (string, error) { return "", errors.New("store closed") })
		if err == nil || errors.Is(err, errLoginLimited) {
			t.Fatal("service failure consumes an attempt")
		}
	}
	if limiter.identity.count != 0 || len(limiter.sources) != 0 {
		t.Fatal("service failure creates state")
	}
}

func TestPeerSourceUsesOnlyTheSocketAddress(t *testing.T) {
	for remote, want := range map[string]string{
		"192.0.2.1:1234": "192.0.2.1", "192.0.2.1:4321": "192.0.2.1",
		"[::ffff:192.0.2.1]:1234": "192.0.2.1", "[2001:db8::1]:1234": "2001:db8::1",
		"hostname:1234": "unknown", "": "unknown", "untrusted": "unknown",
	} {
		if got := peerSource(remote); got != want {
			t.Fatalf("peer %s: %s", remote, got)
		}
	}
}

func TestWebSerializesConcurrentFailedSignIns(t *testing.T) {
	s := sessionStore(t)
	h := s.Handler(false)
	var wait sync.WaitGroup
	var failed, limited atomic.Int32
	for attempt := range 20 {
		r := httptest.NewRequest("POST", "https://localhost/api/v1/session", strings.NewReader(`{"password":"wrong"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("192.0.2.%d", attempt))
		r.Header.Set("Forwarded", fmt.Sprintf("for=192.0.2.%d", attempt))
		addCSRF(t, s, r)
		wait.Add(1)
		go func() {
			defer wait.Done()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			switch w.Code {
			case 401:
				failed.Add(1)
			case 429:
				limited.Add(1)
			}
		}()
	}
	wait.Wait()
	if failed.Load() != 5 || limited.Load() != 15 {
		t.Fatalf("failed=%d limited=%d", failed.Load(), limited.Load())
	}
	if len(s.login.sources) != 1 {
		t.Fatal("forwarded headers create source entries")
	}
}

func TestWebCooldownRejectsCorrectPasswordsUntilTheBoundary(t *testing.T) {
	s := sessionStore(t)
	now := time.Now()
	s.webNow = func() time.Time { return now }
	request := func(password, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://localhost"+path, strings.NewReader(`{"password":"`+password+`"}`))
		r.Header.Set("Content-Type", "application/json")
		addCSRF(t, s, r)
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, r)
		return w
	}
	for range 5 {
		if w := request("wrong-secret", "/api/v1/session"); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	now = now.Add(loginWindow - time.Nanosecond)
	if w := request("synthetic", "/api/v1/session"); w.Code != 429 || w.Header().Get("Retry-After") == "" || strings.Contains(w.Body.String(), "synthetic") {
		t.Fatal("cooldown admits a correct password")
	}
	now = now.Add(time.Nanosecond)
	if w := request("synthetic", "/api/v1/session"); w.Code != 204 {
		t.Fatalf("cooldown boundary: %d %s", w.Code, w.Body)
	}
}

func TestWebIssuesAUsableSessionDuringClockRollback(t *testing.T) {
	s := sessionStore(t)
	now := time.Now()
	s.webNow = func() time.Time { return now }
	request := func(password string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://localhost/api/v1/session", strings.NewReader(`{"password":"`+password+`"}`))
		r.Header.Set("Content-Type", "application/json")
		addCSRF(t, s, r)
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, r)
		return w
	}
	if w := request("wrong"); w.Code != 401 {
		t.Fatalf("first failure: %d", w.Code)
	}
	now = now.Add(-time.Minute)
	w := request("synthetic")
	if w.Code != 204 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("sign-in during rollback: %d %s", w.Code, w.Body)
	}
	if _, err := s.Authenticate(w.Result().Cookies()[0].Value, now); err != nil {
		t.Fatalf("issued session fails validation during rollback: %v", err)
	}
}
