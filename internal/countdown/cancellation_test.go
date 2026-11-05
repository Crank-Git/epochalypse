package countdown

import (
	"context"
	"errors"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/installation"
)

type transactionCancellation struct {
	context.Context
	calls int
}

func (c *transactionCancellation) Err() error {
	c.calls++
	if c.calls >= 2 {
		return context.Canceled
	}
	return nil
}

func TestStoreRollsBackCancellationDuringTransaction(t *testing.T) {
	s, err := Open(t.TempDir(), installation.Limits{MaxObjects: 1, MaxBytes: 8192}, "countdown.example.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := &transactionCancellation{Context: context.Background()}
	if _, err := s.Create(ctx, validInput(), testNow); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled creation commits: %v", err)
	}
	list, err := s.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("canceled creation retains state: %+v %v", list, err)
	}
	r, err := s.Create(context.Background(), validInput(), testNow)
	if err != nil {
		t.Fatal("canceled creation retains quota", err)
	}
	ctx = &transactionCancellation{Context: context.Background()}
	if err := s.Delete(ctx, r.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled deletion commits: %v", err)
	}
	if got, err := s.Get(r.ID); err != nil || got != r {
		t.Fatalf("canceled deletion removes state: %+v %v", got, err)
	}
	if _, err := s.Create(context.Background(), validInput(), testNow); !errors.Is(err, installation.ErrQuota) {
		t.Fatal("canceled deletion releases quota", err)
	}
}
