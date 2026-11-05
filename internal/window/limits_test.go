package window

import (
	"errors"
	"testing"
)

func TestStoreRejectsCreationBeyondConfiguredLimits(t *testing.T) {
	for _, limits := range []Limits{{MaxObjects: 1, MaxBytes: 1 << 30}, {MaxObjects: 100, MaxBytes: 1000}} {
		s, err := OpenWithLimits(t.TempDir(), testSigner(t), limits)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Create(validInput(), testNow); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Create(validInput(), testNow); !errors.Is(err, ErrQuota) {
			t.Fatalf("quota result = %v", err)
		}
		all, err := s.List()
		if err != nil || len(all) != 1 {
			t.Fatal("quota failure changes accepted state")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLimitsRequirePositiveBounds(t *testing.T) {
	for _, limits := range []Limits{{}, {MaxObjects: -1, MaxBytes: 1}, {MaxObjects: 1, MaxBytes: -1}, {MaxObjects: 0, MaxBytes: 1}} {
		if _, err := OpenWithLimits(t.TempDir(), testSigner(t), limits); err == nil {
			t.Fatal("invalid quota passes")
		}
	}
}

func TestStoreRejectsTheFirstDocumentBeyondTheByteQuota(t *testing.T) {
	s, err := OpenWithLimits(t.TempDir(), testSigner(t), Limits{MaxObjects: 1, MaxBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Create(validInput(), testNow); !errors.Is(err, ErrQuota) {
		t.Fatal(err)
	}
	all, err := s.List()
	if err != nil || len(all) != 0 {
		t.Fatal("quota rejection stores a document")
	}
}

func TestStoreAppliesTheObjectQuotaAtomically(t *testing.T) {
	s, err := OpenWithLimits(t.TempDir(), testSigner(t), Limits{MaxObjects: 1, MaxBytes: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := s.Create(validInput(), testNow); results <- err }()
	}
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
		t.Fatal("concurrent creation exceeds the object quota")
	}
}
