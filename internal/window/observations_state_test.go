package window

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/observation"
	bolt "go.etcd.io/bbolt"
)

func observationStore(t *testing.T, limits Limits) (*Store, Window) {
	t.Helper()
	s, err := OpenWithLimits(t.TempDir(), testSigner(t), limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	w, err := s.Create(validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	return s, w
}

func completeResults(run observation.Run) []observation.Result {
	code := uint16(0)
	result := run.Results[0]
	result.Started = testNow
	result.Completed = testNow.Add(time.Second)
	result.Transport = "udp"
	result.Rcode = &code
	result.Flags = &observation.Flags{AD: true, RA: true}
	return []observation.Result{result}
}

func TestObservationReservationBlocksAnotherWindow(t *testing.T) {
	s, w := observationStore(t, Limits{MaxObjects: 2, MaxBytes: 1 << 30})
	run, err := s.ReserveRun(w.ID, observationTargets(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(validInput(), testNow); !errors.Is(err, ErrQuota) {
		t.Fatal("a window bypasses the observation count")
	}
	result, err := s.CompleteRun(w.ID, run.ID, completeResults(run), testNow.Add(2*time.Second))
	if err != nil || result.State != "complete" {
		t.Fatalf("completion fails at the shared quota: %+v: %v", result, err)
	}
	if _, err := s.CompleteRun(w.ID, run.ID, completeResults(run), testNow); !errors.Is(err, ErrRunCompleted) {
		t.Fatal("a completed observation changes")
	}
}

func TestObservationCompletionReleasesOnlyItsByteReservation(t *testing.T) {
	s, w := observationStore(t, DefaultLimits())
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	s.limits.MaxBytes = int64(len(data) + runReservation)
	run, err := s.ReserveRun(w.ID, observationTargets(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(validInput(), testNow); !errors.Is(err, ErrQuota) {
		t.Fatal("a window uses bytes reserved for a pending run")
	}
	if _, err := s.CompleteRun(w.ID, run.ID, completeResults(run), testNow.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(validInput(), testNow); err != nil {
		t.Fatal("completion does not release unused bytes", err)
	}
}

func TestObservationStatePreservesVersionOneWindowsAfterRestart(t *testing.T) {
	directory, signer := t.TempDir(), testSigner(t)
	s, err := Open(directory, signer)
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket(observationBucket) }); err != nil {
		t.Fatal(err)
	}
	s = reopenObservationStore(t, s, directory, signer)
	run, err := s.ReserveRun(w.ID, observationTargets(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	s = reopenObservationStore(t, s, directory, signer)
	defer func() { _ = s.Close() }()
	if err := s.RecoverRuns(testNow.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(w.ID)
	if err != nil || !reflect.DeepEqual(got, w) {
		t.Fatal("the observation migration changes the signed window", err)
	}
	runs, next, err := s.ListRuns(w.ID, "")
	if err != nil || next != "" || len(runs) != 1 {
		t.Fatalf("restart loses a run: %v", err)
	}
	if runs[0].ID != run.ID || runs[0].State != "partial" || runs[0].Results[0].Error != "run_interrupted" {
		t.Fatalf("restart claims a completed query: %+v", runs)
	}
	if _, err := s.CompleteRun(w.ID, run.ID, completeResults(run), testNow); !errors.Is(err, ErrRunCompleted) {
		t.Fatal("restart permits a second completion")
	}
}

func TestObservationPagesBoundThePublicResultCount(t *testing.T) {
	s, w := observationStore(t, DefaultLimits())
	for range 51 {
		if _, err := s.ReserveRun(w.ID, observationTargets(), testNow); err != nil {
			t.Fatal(err)
		}
	}
	first, next, err := s.ListRuns(w.ID, "")
	if err != nil || len(first) != 50 || next == "" {
		t.Fatalf("the first page exceeds its bound: %d %q %v", len(first), next, err)
	}
	last, next, err := s.ListRuns(w.ID, next)
	if err != nil || len(last) != 1 || next != "" {
		t.Fatalf("the last page loses a run: %d %q %v", len(last), next, err)
	}
	if first[49].ID >= last[0].ID {
		t.Fatal("pagination repeats a run")
	}
}

func TestObservationRejectsInvalidReservations(t *testing.T) {
	s, w := observationStore(t, DefaultLimits())
	oversized := observationTargets()
	oversized[0].Version = strings.Repeat("x", runReservation)
	for _, targets := range [][]observation.Target{nil, make([]observation.Target, 4), oversized} {
		if _, err := s.ReserveRun(w.ID, targets, testNow); err == nil {
			t.Fatal("an invalid reservation passes")
		}
	}
	if _, err := s.ReserveRun("invalid", observationTargets(), testNow); err == nil {
		t.Fatal("an invalid window passes")
	}
	if _, err := s.ReserveRun(w.ID, observationTargets(), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("an unencodable time passes")
	}
	s.limits.MaxBytes = runReservation
	if _, err := s.ReserveRun(w.ID, observationTargets(), testNow); !errors.Is(err, ErrQuota) {
		t.Fatal("a pending run bypasses the byte quota")
	}
	runs, _, err := s.ListRuns(w.ID, "")
	if err != nil || len(runs) != 0 {
		t.Fatal("a rejected reservation changes state", err)
	}
}

func TestObservationRejectsResultsOutsideItsReservedContract(t *testing.T) {
	s, w := observationStore(t, DefaultLimits())
	run, err := s.ReserveRun(w.ID, observationTargets(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	changes := []func(*observation.Result){
		func(r *observation.Result) { r.Version = 2 },
		func(r *observation.Result) { r.Name = "other.example." },
		func(r *observation.Result) { r.Resolver.Version = "changed" },
		func(r *observation.Result) { r.Started = time.Time{} },
		func(r *observation.Result) { r.Completed = time.Time{} },
		func(r *observation.Result) { r.Rcode = nil },
		func(r *observation.Result) { r.Flags = nil },
		func(r *observation.Result) { r.Transport = "invalid" },
		func(r *observation.Result) { code := uint16(4096); r.Rcode = &code },
		func(r *observation.Result) { r.Error = "resolver_unavailable" },
		func(r *observation.Result) { r.Rcode = nil; r.Flags = nil; r.Error = "raw secret error" },
	}
	for _, change := range changes {
		results := completeResults(run)
		change(&results[0])
		if _, err := s.CompleteRun(w.ID, run.ID, results, testNow); err == nil {
			t.Fatal("a result changes its reserved contract")
		}
	}
	if _, err := s.CompleteRun(w.ID, run.ID, nil, testNow); err == nil {
		t.Fatal("an incomplete matrix claims completion")
	}
	results := completeResults(run)
	results[0].Rcode = nil
	results[0].Flags = nil
	results[0].Error = "resolver_unavailable"
	completed, err := s.CompleteRun(w.ID, run.ID, results, testNow.Add(2*time.Second))
	if err != nil || completed.State != "partial" {
		t.Fatalf("an unavailable resolver loses its partial result: %+v: %v", completed, err)
	}
}

func TestObservationStoreRejectsUnavailableAndMalformedState(t *testing.T) {
	s, w := observationStore(t, DefaultLimits())
	if _, _, err := s.ListRuns("invalid", ""); err == nil {
		t.Fatal("an invalid window passes")
	}
	if _, _, err := s.ListRuns(w.ID, "invalid"); err == nil {
		t.Fatal("an invalid cursor passes")
	}
	if _, err := s.CompleteRun("invalid", "invalid", nil, testNow); err == nil {
		t.Fatal("invalid identifiers pass")
	}
	if _, err := s.CompleteRun(w.ID, strings.Repeat("0", 32), nil, testNow); !errors.Is(err, ErrNotFound) {
		t.Fatal("a missing run passes")
	}
	key := []byte(w.ID + "/" + strings.Repeat("0", 32))
	if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(observationBucket).Put(key, []byte("invalid")) }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ListRuns(w.ID, ""); err == nil {
		t.Fatal("malformed observation state passes")
	}
	if _, err := s.Create(validInput(), testNow); err == nil {
		t.Fatal("a window ignores malformed quota state")
	}
	if err := s.RecoverRuns(testNow); err == nil {
		t.Fatal("restart ignores malformed observation state")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ListRuns(w.ID, ""); err == nil {
		t.Fatal("a closed store supplies runs")
	}
	if err := s.RecoverRuns(testNow); err == nil {
		t.Fatal("a closed store recovers runs")
	}
	if _, err := s.CompleteRun(w.ID, strings.Repeat("0", 32), nil, testNow); err == nil {
		t.Fatal("a closed store completes a run")
	}
}

func reopenObservationStore(t *testing.T, s *Store, directory string, signer *Signer) *Store {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(directory, signer)
	if err != nil {
		t.Fatal(err)
	}
	return opened
}

func TestObservationVersionOneFixturePreservesUnknownClockAndFailure(t *testing.T) {
	data, err := os.ReadFile("testdata/observation-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := decodeRun(data)
	if err != nil {
		t.Fatal(err)
	}
	result := doc.Run.Results[0]
	if result.Resolver.Clock.OffsetNanoseconds != nil || result.Resolver.Clock.Source != "unknown" || result.Flags != nil || result.Rcode != nil || result.Error != "resolver_unavailable" {
		t.Fatal("the fixture claims unavailable metadata")
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	again, err := decodeRun(encoded)
	if err != nil || !reflect.DeepEqual(doc, again) {
		t.Fatal("the version one fixture changes", err)
	}
}

func TestObservationAndWindowCreationShareOneAtomicQuota(t *testing.T) {
	s, w := observationStore(t, Limits{MaxObjects: 2, MaxBytes: 1 << 30})
	results := make(chan error, 2)
	go func() { _, err := s.Create(validInput(), testNow); results <- err }()
	go func() { _, err := s.ReserveRun(w.ID, observationTargets(), testNow); results <- err }()
	accepted, rejected := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrQuota) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("the shared transaction admits %d creations and %d quota failures", accepted, rejected)
	}
}

func TestObservationPreservesActualTimesAfterBackwardClockChange(t *testing.T) {
	s, w := observationStore(t, DefaultLimits())
	run, err := s.ReserveRun(w.ID, observationTargets(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	results := completeResults(run)
	results[0].Completed = testNow.Add(-time.Minute)
	completed, err := s.CompleteRun(w.ID, run.ID, results, testNow.Add(-time.Minute))
	if err != nil || !completed.Results[0].Completed.Equal(results[0].Completed) {
		t.Fatal("the store changes the observed clock reading", err)
	}
	if err := s.RecoverRuns(testNow); err != nil {
		t.Fatal(err)
	}
	runs, _, err := s.ListRuns(w.ID, "")
	if err != nil || len(runs) != 1 || runs[0].State != "complete" {
		t.Fatal("recovery changes a complete run", err)
	}
}
