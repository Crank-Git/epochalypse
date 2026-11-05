package installation

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func budgetStore(t *testing.T, limits Limits) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func putRecord(t *testing.T, s *Store, bucket string, value []byte) {
	t.Helper()
	if err := s.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return b.Put([]byte("synthetic-record"), value)
	}); err != nil {
		t.Fatal(err)
	}
}

func admits(s *Store, objects int, size int64) error {
	return s.Update(func(tx *bolt.Tx) error { return s.Admit(tx, objects, size) })
}

func TestBudgetCountsEveryDomainWithoutWrappers(t *testing.T) {
	s := budgetStore(t, DefaultLimits())
	window, err := os.ReadFile("../window/testdata/window-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	observation, err := os.ReadFile("../window/testdata/observation-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	capsule := []byte(`{"version":1}`)
	putRecord(t, s, "windows-v1", window)
	putRecord(t, s, "observations-v1", observation)
	putRecord(t, s, "capsules-v1", capsule)
	putRecord(t, s, "capsule-keys-v1", make([]byte, 32))
	putRecord(t, s, "configuration-v1", []byte("fixed DNSKEY configuration"))
	s.Limits = Limits{MaxObjects: 3, MaxBytes: int64(len(window) + len(observation) + len(capsule) + 32)}
	if err := admits(s, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := admits(s, 1, 0); !errors.Is(err, ErrQuota) {
		t.Fatalf("unopened records bypass the count: %v", err)
	}
	if err := admits(s, 0, 1); !errors.Is(err, ErrQuota) {
		t.Fatalf("unopened records bypass the bytes: %v", err)
	}
}

func TestBudgetPreservesPendingObservationReservation(t *testing.T) {
	s := budgetStore(t, Limits{MaxObjects: 2, MaxBytes: ObservationReservationBytes})
	putRecord(t, s, "observations-v1", []byte(`{"run":{"version":1,"state":"pending"},"reserved_bytes":16384}`))
	if err := admits(s, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := admits(s, 0, 1); !errors.Is(err, ErrQuota) {
		t.Fatalf("pending charge shrinks to its JSON size: %v", err)
	}
}

func TestBudgetRejectsCorruptReservationEnvelopes(t *testing.T) {
	for _, value := range []string{
		`not JSON`,
		`{"run":{"version":2,"state":"pending"},"reserved_bytes":16384}`,
		`{"run":{"version":1,"state":"unknown"},"reserved_bytes":16384}`,
		`{"run":{"version":1,"state":"pending"},"reserved_bytes":0}`,
		`{"run":{"version":1,"state":"pending"},"reserved_bytes":-1}`,
		`{"run":{"version":1,"state":"pending"},"reserved_bytes":16385}`,
		`{"run":{"version":1,"state":"pending","completed":"2026-11-04T12:00:00Z"},"reserved_bytes":16384}`,
		`{"run":{"version":1,"state":"complete"},"reserved_bytes":0}`,
		`{"run":{"version":1,"state":"partial","completed":"2026-11-04T12:00:00Z"},"reserved_bytes":16384}`,
	} {
		s := budgetStore(t, DefaultLimits())
		putRecord(t, s, "observations-v1", []byte(value))
		if err := admits(s, 1, 1); err == nil {
			t.Fatalf("corrupt reservation passes: %s", value)
		}
	}
}

func TestBudgetRejectsUnknownStateAndInvalidAdditions(t *testing.T) {
	for _, tc := range []struct{ bucket, value string }{
		{"unknown-v2", `{}`},
		{"windows-v1", `{}`},
		{"windows-v1", `{"version":2}`},
		{"capsules-v1", `{"version":2}`},
		{"capsule-keys-v1", "short key"},
	} {
		s := budgetStore(t, DefaultLimits())
		putRecord(t, s, tc.bucket, []byte(tc.value))
		if err := admits(s, 0, 0); err == nil {
			t.Fatalf("unsupported state passes: %s", tc.bucket)
		}
	}
	s := budgetStore(t, Limits{MaxObjects: 1, MaxBytes: 1})
	for _, tc := range []struct {
		objects int
		size    int64
	}{{-1, 0}, {0, -1}, {2, 0}, {0, 2}} {
		if err := admits(s, tc.objects, tc.size); err == nil {
			t.Fatalf("invalid addition passes: %+v", tc)
		}
	}
	s.Limits.MaxBytes = 0
	if err := admits(s, 0, 0); err == nil {
		t.Fatal("invalid limits pass")
	}
}

func TestBudgetRejectsNestedContentBuckets(t *testing.T) {
	s := budgetStore(t, DefaultLimits())
	if err := s.Update(func(tx *bolt.Tx) error {
		parent, err := tx.CreateBucketIfNotExists([]byte("capsules-v1"))
		if err != nil {
			return err
		}
		_, err = parent.CreateBucket([]byte("nested"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := admits(s, 0, 0); err == nil {
		t.Fatal("a nested content bucket bypasses its charge")
	}
}

func TestBudgetAdmitsOnlyOneConcurrentCreation(t *testing.T) {
	s := budgetStore(t, Limits{MaxObjects: 1, MaxBytes: 1 << 30})
	results := make(chan error, 2)
	for _, name := range []string{"windows-v1", "capsules-v1"} {
		go func() {
			results <- s.Update(func(tx *bolt.Tx) error {
				value := []byte(`{"version":1}`)
				if err := s.Admit(tx, 1, int64(len(value))); err != nil {
					return err
				}
				bucket, err := tx.CreateBucketIfNotExists([]byte(name))
				if err != nil {
					return err
				}
				return bucket.Put([]byte("synthetic-record"), value)
			})
		}()
	}
	accepted, rejected := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrQuota):
			rejected++
		default:
			t.Fatal(err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("admission accepts %d and rejects %d", accepted, rejected)
	}
}

func TestInstallationPreservesLegacyBytesAfterRestart(t *testing.T) {
	directory := t.TempDir()
	s, err := Open(directory, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	fixtures := map[string][]byte{}
	for _, name := range []string{"window", "observation"} {
		data, err := os.ReadFile("../window/testdata/" + name + "-v1.json") // #nosec G304 -- The names come from this fixed fixture list.
		if err != nil {
			t.Fatal(err)
		}
		var encoded any
		if err := json.Unmarshal(data, &encoded); err != nil {
			t.Fatal(err)
		}
		bucket := name + "s-v1"
		fixtures[bucket] = data
		putRecord(t, s, bucket, data)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(directory, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := admits(s, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.View(func(tx *bolt.Tx) error {
		for bucket, original := range fixtures {
			if !bytes.Equal(original, tx.Bucket([]byte(bucket)).Get([]byte("synthetic-record"))) {
				t.Fatal("restart changes fixture bytes")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
