package owner

import (
	"errors"
	"net/netip"
	"sync"
	"time"
)

const loginWindow = 15 * time.Minute
const maxLoginSources = 1024

var errLoginLimited = errors.New("sign-in cooldown")

type loginFailures struct {
	count int
	start time.Time
	until time.Time
}

func (f *loginFailures) expire(now time.Time) {
	if !now.Before(f.start.Add(loginWindow)) && !now.Before(f.until) {
		*f = loginFailures{}
	}
}

func (f *loginFailures) add(now time.Time) {
	if f.count == 0 {
		f.start = now
	}
	f.count++
	if f.count == 5 {
		f.until = now.Add(loginWindow)
	}
}

type loginLimiter struct {
	mu       sync.Mutex
	latest   time.Time
	identity loginFailures
	sources  map[string]loginFailures
}

func peerSource(remote string) string {
	if peer, err := netip.ParseAddrPort(remote); err == nil {
		return peer.Addr().Unmap().String()
	}
	// Unknown peers share one limit instead of accepting attacker-controlled keys.
	return "unknown"
}

func (l *loginLimiter) attempt(source string, now func() time.Time, authenticate func(time.Time) (string, error)) (string, error) {
	// Serialize password checks so concurrent attempts cannot exceed the failure allowance.
	l.mu.Lock()
	defer l.mu.Unlock()
	authenticationTime := now()
	current := authenticationTime
	// A clock rollback must not move the fifth failure's cooldown into the past.
	if current.Before(l.latest) {
		current = l.latest
	}
	l.latest = current
	l.identity.expire(current)
	if current.Before(l.identity.until) {
		return "", errLoginLimited
	}
	if l.sources == nil {
		l.sources = make(map[string]loginFailures)
	}
	for key, failures := range l.sources {
		failures.expire(current)
		if failures.count == 0 {
			delete(l.sources, key)
		}
	}
	failures, exists := l.sources[source]
	if current.Before(failures.until) || (!exists && len(l.sources) >= maxLoginSources) {
		return "", errLoginLimited
	}
	// Session validation uses wall time, so the limiter must not give sessions a future timestamp.
	token, err := authenticate(authenticationTime)
	if errors.Is(err, ErrCredentials) {
		l.identity.add(current)
		failures.add(current)
		l.sources[source] = failures
	}
	return token, err
}
