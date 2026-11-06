package switches

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

var switchTestNow = time.Date(2026, 11, 5, 12, 0, 0, 500000000, time.UTC)

func TestInputUsesDefaultsAndRejectsUnsupportedDurations(t *testing.T) {
	interval, grace, err := (Input{}).durations()
	if err != nil || interval != DefaultIntervalSeconds || grace != DefaultGraceSeconds {
		t.Fatalf("defaults: interval=%d grace=%d err=%v", interval, grace, err)
	}
	for _, input := range []Input{
		{IntervalSeconds: ptrInt64(MinIntervalSeconds), GraceSeconds: ptrInt64(0)},
		{IntervalSeconds: ptrInt64(MaxIntervalSeconds), GraceSeconds: ptrInt64(MaxGraceSeconds)},
	} {
		if gotInterval, gotGrace, err := input.durations(); err != nil || gotInterval != *input.IntervalSeconds || gotGrace != *input.GraceSeconds {
			t.Fatalf("boundary %+v: %d %d %v", input, gotInterval, gotGrace, err)
		}
	}
	for _, input := range []Input{
		{IntervalSeconds: ptrInt64(MinIntervalSeconds - 1)},
		{IntervalSeconds: ptrInt64(MaxIntervalSeconds + 1)},
		{GraceSeconds: ptrInt64(-1)},
		{GraceSeconds: ptrInt64(MaxGraceSeconds + 1)},
	} {
		if _, _, err := input.durations(); !errors.Is(err, ErrInput) {
			t.Fatalf("input %+v: %v", input, err)
		}
	}
}

func TestStoreCreatesVersionedDraftAndReadsItAfterRestart(t *testing.T) {
	dir := t.TempDir()
	shared, s := newDraftStore(t, dir)
	input := Input{IntervalSeconds: ptrInt64(7200), GraceSeconds: ptrInt64(0)}
	record, err := s.Create(context.Background(), input, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if record.Version != 1 || record.ID == "" || record.IntervalSeconds != 7200 || record.GraceSeconds != 0 || record.State != StateDraft || !record.CreatedAt.Equal(switchTestNow.UTC().Truncate(time.Second)) {
		t.Fatalf("record=%+v", record)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}

	_, s = newDraftStore(t, dir)
	got, err := s.Get(record.ID)
	if err != nil || got != record {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	list, err := s.List()
	if err != nil || len(list) != 1 || list[0] != record {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}

func TestStoreRejectsInvalidInputAndTime(t *testing.T) {
	shared, err := installation.Open(t.TempDir(), installation.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shared.Close() }()
	s, err := Attach(shared)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, tc := range []struct {
		input Input
		now   time.Time
	}{{Input{}, time.Time{}}, {Input{IntervalSeconds: ptrInt64(1)}, switchTestNow}, {Input{GraceSeconds: ptrInt64(-1)}, switchTestNow}} {
		if _, err := s.Create(context.Background(), tc.input, tc.now); !errors.Is(err, ErrInput) {
			t.Fatalf("input %+v now %v: %v", tc.input, tc.now, err)
		}
	}
}

func TestStoreRejectsCancelledCreateAndCorruptRecords(t *testing.T) {
	shared, err := installation.Open(t.TempDir(), installation.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shared.Close() }()
	s, err := Attach(shared)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Create(ctx, Input{}, switchTestNow); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	record, err := s.Create(context.Background(), Input{}, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := shared.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(installation.SwitchesBucket)).Put([]byte(record.ID), []byte(`{"version":2}`))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(record.ID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("get corrupt record: %v", err)
	}
	if _, err := s.List(); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("list corrupt record: %v", err)
	}
	if _, err := s.Create(context.Background(), Input{}, switchTestNow); err == nil {
		t.Fatal("admission accepts corrupt retained record")
	}
}

func TestStoreUsesSharedQuotaAndReleasesNoChargeOnFailure(t *testing.T) {
	shared, err := installation.Open(t.TempDir(), installation.Limits{MaxObjects: 1, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shared.Close() }()
	s, err := Attach(shared)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	record, err := s.Create(context.Background(), Input{}, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(context.Background(), Input{}, switchTestNow); !errors.Is(err, installation.ErrQuota) {
		t.Fatalf("second record bypasses installation quota: %v", err)
	}
	if err := shared.Update(func(tx *bolt.Tx) error { return shared.Admit(tx, 1, 0) }); !errors.Is(err, installation.ErrQuota) {
		t.Fatalf("another domain bypasses switch charge: %v", err)
	}
	if _, err := s.Get(record.ID); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRejectsUnknownAndMalformedIDs(t *testing.T) {
	shared, err := installation.Open(t.TempDir(), installation.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shared.Close() }()
	s, err := Attach(shared)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Get("bad"); !errors.Is(err, ErrInput) {
		t.Fatalf("malformed id: %v", err)
	}
	if _, err := s.Get("0123456789abcdef0123456789abcdef"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing id: %v", err)
	}
	list, err := s.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("empty list=%+v err=%v", list, err)
	}
}

func TestRecordValidatorRejectsDraftAuthorityFields(t *testing.T) {
	record := Record{Version: 1, ID: "0123456789abcdef0123456789abcdef", IntervalSeconds: DefaultIntervalSeconds, GraceSeconds: DefaultGraceSeconds, CreatedAt: switchTestNow.UTC().Truncate(time.Second), State: StateDraft}
	if err := ValidateRecord(record); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"version", "id", "interval_seconds", "grace_seconds", "created_at", "state"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("record omits %q: %s", field, encoded)
		}
	}
	for _, forbidden := range []string{"deadline", "proposed_at", "agent", "receipts"} {
		if _, ok := fields[forbidden]; ok {
			t.Fatalf("draft retains %q: %s", forbidden, encoded)
		}
	}
	for _, invalid := range []Record{
		{Version: 2, ID: record.ID, IntervalSeconds: record.IntervalSeconds, GraceSeconds: record.GraceSeconds, CreatedAt: record.CreatedAt, State: StateDraft},
		{Version: 1, ID: record.ID, IntervalSeconds: record.IntervalSeconds, GraceSeconds: record.GraceSeconds, CreatedAt: record.CreatedAt, State: "armed"},
		{Version: 1, ID: record.ID, IntervalSeconds: record.IntervalSeconds, GraceSeconds: record.GraceSeconds, CreatedAt: time.Time{}, State: StateDraft},
		{Version: 1, ID: record.ID, IntervalSeconds: 1, GraceSeconds: record.GraceSeconds, CreatedAt: record.CreatedAt, State: StateDraft},
	} {
		if !errors.Is(ValidateRecord(invalid), ErrIntegrity) {
			t.Fatalf("accepted invalid record: %+v", invalid)
		}
	}
}

func ptrInt64(value int64) *int64 { return &value }

func TestStoreRejectsClosedDatabaseAndUnknownRecordFields(t *testing.T) {
	if _, err := Attach(nil); !errors.Is(err, ErrInput) {
		t.Fatalf("nil installation: %v", err)
	}
	shared, err := installation.Open(t.TempDir(), installation.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	s, err := Attach(shared)
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.Create(context.Background(), Input{}, switchTestNow)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{append(append([]byte{}, data[:len(data)-1]...), []byte(`,"deadline":"2026-11-05T12:00:00Z"}`)...), []byte(strings.Repeat("x", 4097))} {
		if err := shared.Update(func(tx *bolt.Tx) error {
			return tx.Bucket([]byte(installation.SwitchesBucket)).Put([]byte(record.ID), bad)
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(record.ID); !errors.Is(err, ErrIntegrity) {
			t.Fatalf("invalid retained fields: %v", err)
		}
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Attach(shared); err == nil {
		t.Fatal("closed installation accepts an attachment")
	}
	if _, err := s.Create(context.Background(), Input{}, switchTestNow); err == nil {
		t.Fatal("closed installation accepts a draft")
	}
	if _, err := s.List(); err == nil {
		t.Fatal("closed installation returns a list")
	}
	_ = s.Close()
}

func newDraftStore(t *testing.T, directory string) (*installation.Store, *Store) {
	t.Helper()
	shared, err := installation.Open(directory, installation.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Attach(shared)
	if err != nil {
		_ = shared.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close(); _ = shared.Close() })
	return shared, store
}
