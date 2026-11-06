package agent

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestAuthorityRemovesTemporaryCheckpointAfterWriteFailure(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	j, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected checkpoint write failure")
	j.performIO = func(name string, action func() error) error {
		if name == "write-checkpoint" {
			return failure
		}
		return action()
	}
	if _, err := j.Commit(authorityTestState()); !errors.Is(err, failure) {
		t.Fatalf("Commit error = %v, want checkpoint write cause", err)
	}
	if _, err := j.Snapshot(); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("Snapshot after uncertain checkpoint write = %v, want quarantine", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(config.CheckpointDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".checkpoint-") {
			t.Fatalf("failed checkpoint leaves temporary file %q", entry.Name())
		}
	}

	restarted, err := Open(config)
	if err != nil {
		t.Fatalf("Open after checkpoint write failure: %v", err)
	}
	defer closeAuthorityTestJournal(t, restarted)
	state, err := restarted.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if state.Generation != 1 || state.Phase != authorityTestState().Phase {
		t.Fatalf("replayed state = generation %d, phase %d", state.Generation, state.Phase)
	}
}

func TestAuthorityRejectsCheckpointAliasIntoAuthorityVolume(t *testing.T) {
	config := authorityTestConfig(t)
	alias := config.Directory + "-checkpoint"
	if err := os.Symlink(config.Directory, alias); err != nil {
		t.Fatal(err)
	}
	config.CheckpointDirectory = alias
	if err := Initialize(config); err == nil {
		t.Fatal("Initialize accepts a checkpoint alias inside the authority volume")
	}
	entries, err := os.ReadDir(config.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected checkpoint alias changes authority: %v", entries)
	}
}

func TestOpenRejectsAuthorityLockSymlink(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	lockPath := config.Directory + "/" + authorityLockName
	if err := os.Symlink(config.Directory+"/missing-lock-target", lockPath); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if journal != nil {
		_ = journal.Close()
	}
	if err == nil {
		t.Fatal("Open accepts a symbolic link as the authority lock")
	}
	if _, err := os.Lstat(lockPath); err != nil {
		t.Fatalf("Open changes the rejected lock path: %v", err)
	}
}

func TestReadBoundedFileChecksFileTypeAndSize(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(directory+"/small", []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directory+"/large", []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory+"/nested", 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	data, err := readBoundedFile(root, "small", 3)
	if err != nil || string(data) != "abc" {
		t.Fatalf("readBoundedFile small = %q, %v", data, err)
	}
	if _, err := readBoundedFile(root, "missing", 3); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file error = %v", err)
	}
	if _, err := readBoundedFile(root, "nested", 3); err == nil {
		t.Fatal("readBoundedFile accepts a directory")
	}
	if _, err := readBoundedFile(root, "large", 3); !errors.Is(err, ErrRecordLimit) {
		t.Fatalf("oversized file error = %v, want ErrRecordLimit", err)
	}
}

func TestAuthorityHelperHandlesClosedAndNilResources(t *testing.T) {
	if err := releaseAuthorityLock(nil); err != nil {
		t.Fatalf("releaseAuthorityLock(nil) = %v", err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if err := syncRoot(root); err == nil {
		t.Fatal("syncRoot accepts a closed root")
	}
}
