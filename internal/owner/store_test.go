package owner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnerSurvivesRestartAndRecovery(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Verify([]byte("synthetic")); !errors.Is(err, ErrCredentials) {
		t.Fatalf("missing owner: %v", err)
	}
	if err := store.Initialize([]byte("synthetic-old")); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := store.Verify([]byte("synthetic-old")); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify([]byte("wrong")); !errors.Is(err, ErrCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if err := store.Recover([]byte("synthetic-new")); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify([]byte("synthetic-old")); !errors.Is(err, ErrCredentials) {
		t.Fatalf("old password: %v", err)
	}
	if err := store.Verify([]byte("synthetic-new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(directory, "v1", "owner.db"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("database mode: %v", info.Mode())
	}
}

func TestPasswordRejectsUnboundedProfile(t *testing.T) {
	first, err := newPassword([]byte("synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := newPassword([]byte("synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Salt == second.Salt {
		t.Fatal("password salt repeats")
	}
	first.Memory = ^uint32(0)
	if first.matches([]byte("synthetic")) {
		t.Fatal("invalid cost accepted")
	}
}

func BenchmarkPasswordProfile(b *testing.B) {
	for b.Loop() {
		if _, err := newPassword([]byte("synthetic-benchmark")); err != nil {
			b.Fatal(err)
		}
	}
}
