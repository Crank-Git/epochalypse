package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicationRejectsInvalidIdentityBeforeStage(t *testing.T) {
	for _, identity := range [][2]int{{-1, os.Getgid()}, {os.Getuid(), -1}} {
		parent := t.TempDir()
		err := fixture(t).publish(context.Background(), filepath.Join(parent, "restored"), identity[0], identity[1])
		if err == nil || !strings.Contains(err.Error(), "numeric service identity") {
			t.Fatalf("invalid identity: %v", err)
		}
		assertEmpty(t, parent)
	}
}

func TestPublicationRejectsMissingParentWithoutStage(t *testing.T) {
	parent := t.TempDir()
	err := fixture(t).publish(context.Background(), filepath.Join(parent, "missing", "restored"), os.Getuid(), os.Getgid())
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing publication parent: %v", err)
	}
	assertEmpty(t, parent)
}

func TestInstallFilePreservesExistingDestination(t *testing.T) {
	parent := t.TempDir()
	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	file := fixture(t).Files[0]
	file.Path = "existing"
	if err := root.WriteFile(file.Path, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := installFile(context.Background(), root, file, os.Getuid(), os.Getgid()); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing publication file: %v", err)
	}
	data, err := root.ReadFile(file.Path)
	if err != nil || string(data) != "retained" {
		t.Fatalf("existing file changes: %q %v", data, err)
	}
}

func TestSyncDirectoriesRejectsMissingState(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := syncDirectories(root, os.Getuid(), os.Getgid()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("incomplete publication directories: %v", err)
	}
}

func TestAuthenticatedInstallationRejectsMissingStage(t *testing.T) {
	a := fixture(t)
	a.stage = filepath.Join(t.TempDir(), "missing")
	if err := a.validateFiles(context.Background()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing authenticated file stage: %v", err)
	}
}

func TestInstallationValidationRejectsLostStage(t *testing.T) {
	a := installationArchive(t, nil)
	settings, signer, err := a.validateConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	a.stage = filepath.Join(t.TempDir(), "missing")
	if err := a.validateInstallation(context.Background(), settings, signer); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing installation stage loses cause: %v", err)
	}
}

func TestInstallationValidationCancelsDatabaseAuthentication(t *testing.T) {
	a := installationArchive(t, nil)
	settings, signer, err := a.validateConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.validateInstallation(ctx, settings, signer); !errors.Is(err, context.Canceled) {
		t.Fatalf("database authentication loses cancellation: %v", err)
	}
	if err := a.validateInstallation(context.Background(), settings, signer); err != nil {
		t.Fatalf("cancelled database authentication retains its lock: %v", err)
	}
}
