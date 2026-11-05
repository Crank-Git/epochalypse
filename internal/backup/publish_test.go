package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPublicationPreservesEveryFile(t *testing.T) {
	a := fixture(t)
	destination := filepath.Join(t.TempDir(), "restored")
	if err := a.publish(context.Background(), destination, os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	for _, file := range a.Files {
		data, err := os.ReadFile(filepath.Join(destination, file.Path)) // #nosec G304 -- The fixture selects the destination and allowlisted paths.
		if err != nil || string(data) != string(readContent(t, file)) {
			t.Fatalf("restored file differs: %v", err)
		}
		info, err := os.Stat(filepath.Join(destination, file.Path))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("restored file lacks private permissions")
		}
	}
}

func TestPublicationPreservesExistingDestination(t *testing.T) {
	for _, populated := range []bool{false, true} {
		a := fixture(t)
		destination := t.TempDir()
		if populated {
			if err := os.WriteFile(filepath.Join(destination, "current"), []byte("current"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		before, err := os.Stat(destination)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.publish(context.Background(), destination, os.Getuid(), os.Getgid()); err == nil {
			t.Fatal("publication replaces an existing destination")
		}
		after, err := os.Stat(destination)
		if err != nil || !os.SameFile(before, after) {
			t.Fatal("publication changes the destination")
		}
	}
}

func TestPublicationCancelsWithoutDestination(t *testing.T) {
	a := fixture(t)
	parent := t.TempDir()
	destination := filepath.Join(parent, "restored")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.publish(ctx, destination, os.Getuid(), os.Getgid()); !errors.Is(err, context.Canceled) {
		t.Fatalf("publication ignores cancellation: %v", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("publication leaves partial state")
	}
}

func TestPublicationRejectsChangedSourceWithoutDestination(t *testing.T) {
	a := fixture(t)
	if err := os.WriteFile(a.Files[0].SourcePath, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	if err := a.publish(context.Background(), filepath.Join(parent, "restored"), os.Getuid(), os.Getgid()); err == nil {
		t.Fatal("publication accepts changed source bytes")
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("publication leaves partial state")
	}
}
