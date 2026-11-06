package agent

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestReleaseWithholdsShareAfterDurabilityFailure(t *testing.T) {
	for _, operation := range []string{"seek-journal", "write-record-body", "sync-record-body", "write-commit-marker", "sync-commit-marker", "write-checkpoint", "sync-checkpoint", "rename-checkpoint", "sync-checkpoint-directory"} {
		t.Run(operation, func(t *testing.T) {
			f, tr, _ := armedCheckinFixture(t)
			tr.clock = func() (Reading, error) { return Reading{lower: 4660}, nil }
			fault := errors.New("synthetic release durability failure")
			installDurabilityFault(f.journal, operation, fault)
			if err := tr.Publish(); !errors.Is(err, fault) {
				t.Fatalf("release loses failure: %v", err)
			}
			w := httptest.NewRecorder()
			tr.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/agent/v1/public/switches/switch-1", nil))
			if w.Code != http.StatusServiceUnavailable || bytes.Contains(w.Body.Bytes(), []byte("release")) {
				t.Fatalf("failed publication exposes release: %d %s", w.Code, w.Body)
			}
			if err := f.journal.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(f.authorityConfig)
			if errors.Is(err, ErrQuarantined) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = reopened.Close() }()
			config := f.config(0)
			config.Journal = reopened
			config.ReadClock = tr.clock
			retry, err := NewTransitions(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := retry.Publish(); err != nil {
				t.Fatalf("intact release cannot retry: %v", err)
			}
			state, err := reopened.Snapshot()
			if err != nil || !state.PossibleDisclosure {
				t.Fatalf("restart loses release: %v", err)
			}
		})
	}
}

func TestDelayedCheckinReleasesAtSignedBoundary(t *testing.T) {
	f, tr, frame := armedCheckinFixture(t)
	request, err := switchwire.SignFrame(frame, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	tr.clock = func() (Reading, error) { return Reading{lower: 4500}, nil }
	if _, err := tr.Checkin(request); err != nil {
		t.Fatal(err)
	}
	for _, now := range []int64{4759, 4760} {
		tr.clock = func() (Reading, error) { return Reading{lower: now}, nil }
		if err := tr.Publish(); err != nil {
			t.Fatal(err)
		}
		state, err := f.journal.Snapshot()
		if err != nil || state.PossibleDisclosure != (now == 4760) {
			t.Fatalf("arrival changes release at %d: %v", now, err)
		}
	}
}

func TestReleaseSerializesWithConcurrentCheckin(t *testing.T) {
	f, tr, frame := armedCheckinFixture(t)
	request, err := switchwire.SignFrame(frame, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	tr.clock = func() (Reading, error) { return Reading{lower: 4660}, nil }
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(2)
	go func() { defer group.Done(); <-start; _ = tr.Publish() }()
	go func() { defer group.Done(); <-start; _, _ = tr.Checkin(request) }()
	close(start)
	group.Wait()
	state, err := f.journal.Snapshot()
	if err != nil || !state.PossibleDisclosure {
		t.Fatalf("boundary race suppresses release: %v", err)
	}
	if _, err := tr.Checkin(request); err == nil {
		t.Fatal("released race accepts renewal")
	}
}

func TestReleaseWithholdsShareAfterUnsafeClock(t *testing.T) {
	f, tr, _ := armedCheckinFixture(t)
	tr.clock = func() (Reading, error) { return Reading{}, ErrClock }
	if err := tr.Publish(); !errors.Is(err, ErrClock) {
		t.Fatalf("unsafe clock release: %v", err)
	}
	state, err := f.journal.Snapshot()
	if err != nil || state.PossibleDisclosure || !state.UnsafeClock {
		t.Fatalf("unsafe clock state: %v", err)
	}
	tr.clock = func() (Reading, error) { return Reading{lower: 9000}, nil }
	if err := tr.Publish(); err == nil {
		t.Fatal("later clock clears quarantine")
	}
}

func TestPublisherReleasesAtBoundaryBeforeContextShutdown(t *testing.T) {
	f, tr, _ := armedCheckinFixture(t)
	tr.clock = func() (Reading, error) { return Reading{lower: 4660}, nil }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tr.RunPublisher(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("publisher shutdown: %v", err)
	}
	state, err := f.journal.Snapshot()
	if err != nil || !state.PossibleDisclosure {
		t.Fatalf("publisher misses eligible release: %v", err)
	}
}
