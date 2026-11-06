package authority

import (
	"context"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/installation"
)

func TestCountdownAuthorityWithholdsAnswersAfterStoreClose(t *testing.T) {
	a, store := countdownAuthorityFixture(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reply := query(t, a, a.zone, dns.TypeSOA, true, false, now)
	if reply.Rcode != dns.RcodeServerFailure || len(reply.Answer) != 0 {
		t.Fatalf("closed countdown authority: %s", reply)
	}
}

func TestCountdownAuthorityRejectsUnsupportedSignatureTime(t *testing.T) {
	for _, populated := range []bool{false, true} {
		a, store := countdownAuthorityFixture(t)
		if populated {
			if _, err := store.Create(context.Background(), countdown.Input{Title: "Synthetic event", EventAt: now.Add(time.Hour).Format(time.RFC3339)}, now); err != nil {
				t.Fatal(err)
			}
		}
		reply := query(t, a, a.zone, dns.TypeSOA, true, false, time.Unix(-3600, 0))
		if reply.Rcode != dns.RcodeServerFailure || len(reply.Answer) != 0 {
			t.Fatalf("unsupported countdown clock: %s", reply)
		}
	}
}

func TestCountdownAuthorityOmitsRetainedPreviousZone(t *testing.T) {
	dir := t.TempDir()
	store, err := countdown.Open(dir, installation.DefaultLimits(), "previous.example.")
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.Create(context.Background(), countdown.Input{Title: "Retained event", EventAt: now.Add(time.Hour).Format(time.RFC3339)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	a, _ := countdownAuthorityFixture(t)
	store, err = countdown.Open(dir, installation.DefaultLimits(), a.zone)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	a.countdowns = store
	snapshot, err := a.current(now)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := snapshot.nodes[old.Name]; exists || len(snapshot.names) != 1 {
		t.Fatal("new zone exposes the retained previous-zone record")
	}
	if retained, err := store.Get(old.ID); err != nil || retained != old {
		t.Fatalf("zone change alters retained record: %v", err)
	}
}
