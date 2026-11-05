package capsule

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
)

var capsuleNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func capsuleInput(mode Mode) Input {
	return Input{Mode: mode, Kind: Text, Title: "Synthetic capsule", ReleaseAt: capsuleNow.Add(time.Hour)}
}

func capsuleStore(t *testing.T, limit int) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), installation.DefaultLimits(), limit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestServerHeldDraftWithholdsObjectAndKeyUntilPublication(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Published(record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("draft exposes an object: %v", err)
	}
	if key, err := s.Key(record.ID, capsuleNow.Add(2*time.Hour)); !errors.Is(err, ErrNotFound) || key != [32]byte{} {
		t.Fatalf("draft exposes a key: %v", err)
	}
}

func TestServerHeldKeyUsesExactReleaseBoundary(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	published, err := s.Publish(record.ID, capsuleNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if published.EffectiveReleaseAt == nil || !published.EffectiveReleaseAt.Equal(record.RequestedReleaseAt) {
		t.Fatal("server-held publication changes its release time")
	}
	if key, err := s.Key(record.ID, record.RequestedReleaseAt.Add(-time.Nanosecond)); !errors.Is(err, ErrTooEarly) || key != [32]byte{} {
		t.Fatalf("early key: %v", err)
	}
	key, err := s.Key(record.ID, record.RequestedReleaseAt)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := OpenObject(record.ID, record.Version, published.Object, key, 32)
	if err != nil || string(plaintext) != "private text" {
		t.Fatalf("released key cannot authenticate content: %v", err)
	}
}

func TestPublicModeExposesKeyAtPublication(t *testing.T) {
	s := capsuleStore(t, 32)
	record, err := s.Create(capsuleInput(Public), strings.NewReader("public text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	if record.EffectiveReleaseAt != nil {
		t.Fatal("a private public-mode draft claims an effective publication time")
	}
	publishedAt := capsuleNow.Add(time.Minute)
	published, err := s.Publish(record.ID, publishedAt)
	if err != nil {
		t.Fatal(err)
	}
	if published.EffectiveReleaseAt == nil || !published.EffectiveReleaseAt.Equal(publishedAt) || !published.RequestedReleaseAt.Equal(record.RequestedReleaseAt) {
		t.Fatal("public publication loses its actual or requested time")
	}
	key, err := s.Key(record.ID, publishedAt)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := OpenObject(record.ID, record.Version, published.Object, key, 32)
	if err != nil || string(plaintext) != "public text" {
		t.Fatalf("public key cannot authenticate content: %v", err)
	}
}

func TestCreateReadsAtMostOneByteBeyondPlaintextLimit(t *testing.T) {
	s := capsuleStore(t, 4)
	if _, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("12345"), capsuleNow); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversized content passes: %v", err)
	}
	if _, err := s.Create(capsuleInput(ServerHeld), strings.NewReader("1234"), capsuleNow); err != nil {
		t.Fatalf("exact limit fails: %v", err)
	}
}
