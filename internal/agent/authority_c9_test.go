package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthorityRejectsCheckpointSymlinkCycleBeforeInitialization(t *testing.T) {
	config := authorityTestConfig(t)
	cycle := filepath.Join(t.TempDir(), "cycle")
	if err := os.Symlink(cycle, cycle); err != nil {
		t.Fatal(err)
	}
	config.CheckpointDirectory = filepath.Join(cycle, "checkpoint")
	if err := Initialize(config); err == nil || !strings.Contains(err.Error(), "resolve checkpoint directory") {
		t.Fatalf("cyclic checkpoint path initializes authority: %v", err)
	}
	entries, err := os.ReadDir(config.Directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected checkpoint path changes authority: %v, %v", entries, err)
	}
}

func TestAuthorityRejectsVolumeAliasBeforeInitialization(t *testing.T) {
	config := authorityTestConfig(t)
	realDirectory := config.Directory
	alias := filepath.Join(t.TempDir(), "volume-alias")
	if err := os.Symlink(realDirectory, alias); err != nil {
		t.Fatal(err)
	}
	config.Directory = alias
	if err := Initialize(config); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("aliased volume initializes authority: %v", err)
	}
	entries, err := os.ReadDir(realDirectory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected alias changes authority: %v, %v", entries, err)
	}
}

func TestOpenPreservesMissingVolumeMarkerAndPermitsRepair(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(config.Directory, authorityMarkerName)
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if journal != nil {
		_ = journal.Close()
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing marker error loses its cause: %v", err)
	}
	if err := os.WriteFile(marker, []byte(config.ExpectedVolumeID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(config)
	if err != nil {
		t.Fatalf("failed open retains an authority lock: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}
