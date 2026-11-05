package countdown

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

var testNow = time.Date(2026, 11, 5, 12, 0, 0, 500000000, time.UTC)

func validInput() Input {
	return Input{Title: "Synthetic event", EventAt: testNow.Add(time.Hour).Truncate(time.Second).Format(time.RFC3339)}
}

func TestInputRejectsUnsupportedEventTimes(t *testing.T) {
	for _, value := range []string{"", "tomorrow", testNow.Truncate(time.Second).Format(time.RFC3339), testNow.Add(-time.Hour).Format(time.RFC3339), testNow.Add(time.Hour).Format(time.RFC3339Nano), "2026-11-05T13:00:00+00:00", testNow.Add((MaxTTL + 1) * time.Second).Truncate(time.Second).Format(time.RFC3339)} {
		in := validInput()
		in.EventAt = value
		if _, err := in.Validate(testNow); !errors.Is(err, ErrInput) {
			t.Fatalf("event %q: %v", value, err)
		}
	}
	for _, title := range []string{"", "  ", strings.Repeat("x", 129), string([]byte{255})} {
		in := validInput()
		in.Title = title
		if _, err := in.Validate(testNow); !errors.Is(err, ErrInput) {
			t.Fatalf("title %q: %v", title, err)
		}
	}
	in := validInput()
	in.EventAt = testNow.Add(MaxTTL * time.Second).Truncate(time.Second).Format(time.RFC3339)
	if _, err := in.Validate(testNow); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRetainsEventUntilDeletionAfterRestart(t *testing.T) {
	directory := t.TempDir()
	s, err := Open(directory, installation.DefaultLimits(), "countdown.example.")
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.Create(context.Background(), validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	assertStaticEventRecord(t, record)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(directory, installation.DefaultLimits(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	got, err := s.Get(record.ID)
	if err != nil || got != record {
		t.Fatalf("restart: %+v %v", got, err)
	}
	list, err := s.List()
	if err != nil || len(list) != 1 || list[0] != record {
		t.Fatalf("list: %+v %v", list, err)
	}
	if _, err := s.Create(context.Background(), validInput(), testNow); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func assertStaticEventRecord(t *testing.T, record Record) {
	t.Helper()
	if record.Name != "c"+record.ID+".countdown.example." || record.TXT().Txt[0] != "v=epochalypse1 event="+validInput().EventAt || record.TXT().Hdr.TTL != MaxTTL {
		t.Fatalf("record=%+v TXT=%s", record, record.TXT())
	}
}

func TestStoreAdmitsOnlyOneConcurrentObject(t *testing.T) {
	shared, err := installation.Open(t.TempDir(), installation.Limits{MaxObjects: 1, MaxBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shared.Close() }()
	s, err := Attach(shared, "countdown.example.")
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := s.Create(context.Background(), validInput(), testNow)
			results <- err
		}()
	}
	group.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, installation.ErrQuota) {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted=%d", accepted)
	}
	if err := shared.Update(func(tx *bolt.Tx) error { return shared.Admit(tx, 1, 1) }); !errors.Is(err, installation.ErrQuota) {
		t.Fatalf("other domain admission: %v", err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), list[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := shared.Update(func(tx *bolt.Tx) error { return shared.Admit(tx, 1, 1) }); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if err := shared.View(func(*bolt.Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(list[0].ID); err == nil {
		t.Fatal("closed domain remains readable")
	}
}

func TestStoreChargesExactEncodedBytesBeforeWrite(t *testing.T) {
	shared, err := installation.Open(t.TempDir(), installation.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shared.Close() }()
	s, err := Attach(shared, "countdown.example.")
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.Create(context.Background(), validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), record.ID); err != nil {
		t.Fatal(err)
	}
	shared.Limits.MaxBytes = int64(len(encoded) - 1)
	if _, err := s.Create(context.Background(), validInput(), testNow); !errors.Is(err, installation.ErrQuota) {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("failed write retained: %+v %v", list, err)
	}
	shared.Limits.MaxBytes++
	if _, err := s.Create(context.Background(), validInput(), testNow); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRejectsCancellationAndCorruption(t *testing.T) {
	shared, err := installation.Open(t.TempDir(), installation.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shared.Close() }()
	s, err := Attach(shared, "countdown.example.")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Create(ctx, validInput(), testNow); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	record, err := s.Create(context.Background(), validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, record.ID); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := shared.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(installation.CountdownsBucket)).Put([]byte(record.ID), []byte(`{"version":2}`))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(record.ID); !errors.Is(err, ErrIntegrity) {
		t.Fatal(err)
	}
	if _, err := s.List(); !errors.Is(err, ErrIntegrity) {
		t.Fatal(err)
	}
	if _, err := s.Create(context.Background(), validInput(), testNow); err == nil {
		t.Fatal("admission ignores invalid retained version")
	}
}
