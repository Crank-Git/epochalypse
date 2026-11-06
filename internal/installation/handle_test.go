package installation

import (
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestDomainHandleClosePreservesOtherDomains(t *testing.T) {
	s := budgetStore(t, DefaultLimits())
	first, second := s.Handle(), s.Handle()
	if err := first.Update(func(tx *bolt.Tx) error { return first.Admit(tx, 1, 1) }); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.View(func(*bolt.Tx) error { t.Fatal("a closed domain reads content"); return nil }); err == nil {
		t.Fatal("a closed domain permits reads")
	}
	if err := first.Update(func(*bolt.Tx) error { t.Fatal("a closed domain changes content"); return nil }); err == nil {
		t.Fatal("a closed domain permits writes")
	}
	if err := second.View(func(*bolt.Tx) error { return nil }); err != nil {
		t.Fatalf("one domain closes another: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Update(func(*bolt.Tx) error { return nil }); err == nil {
		t.Fatal("installation shutdown preserves a writer")
	}
}

func TestBudgetRejectsOverflowingRetainedBytes(t *testing.T) {
	s := budgetStore(t, Limits{MaxObjects: int(^uint(0) >> 1), MaxBytes: 1<<63 - 1})
	putRecord(t, s, CapsuleKeysBucket, make([]byte, 32))
	if err := admits(s, 0, 1<<63-1); err == nil {
		t.Fatal("retained bytes overflow the installation allowance")
	}
}
