package resolver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/observation"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func jobsFixture(t *testing.T, limits ...window.Limits) (*window.Store, window.Window, []observation.Target) {
	t.Helper()
	key := dns.NewDNSKEY("windows.example.", dns.ECDSAP256SHA256)
	key.Flags = 257
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic.private")
	for path, text := range map[string]string{path: key.PrivateKeyString(private), strings.TrimSuffix(path, ".private") + ".key": key.String()} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	signer, err := window.LoadSigner("windows.example.", path)
	if err != nil {
		t.Fatal(err)
	}
	limit := window.DefaultLimits()
	if len(limits) > 0 {
		limit = limits[0]
	}
	store, err := window.OpenWithLimits(t.TempDir(), signer, limit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now().UTC().Truncate(time.Second)
	w, err := store.Create(window.Input{Content: "public job experiment", Inception: now.Add(time.Hour).Format(time.RFC3339), Expiration: now.Add(2 * time.Hour).Format(time.RFC3339)}, now)
	if err != nil {
		t.Fatal(err)
	}
	targets := []observation.Target{{ID: "strict", Address: "127.0.0.1:1", Version: "unknown", VersionSource: "unknown", Clock: observation.Clock{Source: "unknown"}}}
	return store, w, targets
}

func TestJobsPersistResultsBeforeShutdownReturns(t *testing.T) {
	store, w, targets := jobsFixture(t)
	jobs, err := NewJobs(store, targets)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(jobs.Close)
	run, err := jobs.Start(w.ID)
	if err != nil || run.State != "pending" || run.WindowID != w.ID {
		t.Fatalf("admission does not reserve the run: %+v: %v", run, err)
	}
	jobs.Close()
	runs, _, err := store.ListRuns(w.ID, "")
	if err != nil || len(runs) != 1 || runs[0].State != "partial" || runs[0].Results[0].Error != "resolver_unavailable" {
		t.Fatalf("shutdown leaves an unfinished run: %+v: %v", runs, err)
	}
	if _, err := jobs.Start(w.ID); !errors.Is(err, ErrUnavailable) {
		t.Fatal("a closed job service admits work")
	}
}

func TestJobsRejectMissingOrInvalidConfiguration(t *testing.T) {
	store, _, targets := jobsFixture(t)
	if _, err := NewJobs(nil, targets); err == nil {
		t.Fatal("jobs accept an absent store")
	}
	if _, err := NewJobs(store, nil); err == nil {
		t.Fatal("jobs accept absent resolver targets")
	}
}

func TestJobsBoundAdmissionAndDrainEveryAcceptedRun(t *testing.T) {
	store, w, targets := jobsFixture(t)
	jobs, err := NewJobs(store, targets)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 4)
	jobs.query = func(ctx context.Context, name string, target observation.Target) observation.Result {
		entered <- struct{}{}
		<-ctx.Done()
		return Observe(ctx, name, target)
	}
	for range 4 {
		if _, err := jobs.Start(w.ID); err != nil {
			t.Fatal(err)
		}
	}
	for range 4 {
		<-entered
	}
	if _, err := jobs.Start(w.ID); !errors.Is(err, ErrBusy) {
		t.Fatal("a fifth job bypasses the work bound")
	}
	jobs.Close()
	runs, _, err := store.ListRuns(w.ID, "")
	if err != nil || len(runs) != 4 {
		t.Fatal("admission loses a durable run", err)
	}
	for _, run := range runs {
		if run.State != "partial" {
			t.Fatal("shutdown does not drain each accepted run")
		}
	}
}

func TestJobsReleaseAdmissionAfterQuotaFailure(t *testing.T) {
	store, w, targets := jobsFixture(t, window.Limits{MaxObjects: 1, MaxBytes: 1 << 30})
	jobs, err := NewJobs(store, targets)
	if err != nil {
		t.Fatal(err)
	}
	defer jobs.Close()
	for range 5 {
		if _, err := jobs.Start(w.ID); !errors.Is(err, window.ErrQuota) {
			t.Fatal("a rejected reservation consumes admission", err)
		}
	}
	if !jobs.Available() || len(jobs.slots) != 0 {
		t.Fatal("a quota failure leaves active work")
	}
}

func TestJobsFailClosedAfterDurableCompletionFails(t *testing.T) {
	store, w, targets := jobsFixture(t)
	jobs, err := NewJobs(store, targets)
	if err != nil {
		t.Fatal(err)
	}
	jobs.query = func(ctx context.Context, name string, target observation.Target) observation.Result {
		_ = store.Close()
		return Observe(ctx, name, target)
	}
	if _, err := jobs.Start(w.ID); err != nil {
		t.Fatal(err)
	}
	jobs.wg.Wait()
	if jobs.Available() {
		t.Fatal("a failed completion claims available results")
	}
	if _, err := jobs.Start(w.ID); !errors.Is(err, ErrUnavailable) {
		t.Fatal("a failed service admits another job")
	}
	jobs.Close()
}

func TestJobsCopyConfiguredClockOffsets(t *testing.T) {
	store, w, targets := jobsFixture(t)
	offset, lower, upper := int64(-2000000000), int64(-2100000000), int64(-1900000000)
	targets[0].Clock = observation.Clock{Source: "configured", OffsetNanoseconds: &offset, LowerNanoseconds: &lower, UpperNanoseconds: &upper}
	jobs, err := NewJobs(store, targets)
	if err != nil {
		t.Fatal(err)
	}
	offset = 0
	lower = 0
	upper = 0
	run, err := jobs.Start(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	jobs.Close()
	clock := run.Results[0].Resolver.Clock
	if *clock.OffsetNanoseconds != -2000000000 || *clock.LowerNanoseconds != -2100000000 || *clock.UpperNanoseconds != -1900000000 {
		t.Fatal("caller mutation changes the configured clock metadata")
	}
}
