package agent

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthorityClosePreservesBothResourceFailures(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := journal.lock.Close(); err != nil {
		t.Fatal(err)
	}
	err = journal.Close()
	if err == nil || !strings.Contains(err.Error(), "close authority journal") || !strings.Contains(err.Error(), "unlock authority volume") {
		t.Fatalf("Close discards a resource failure: %v", err)
	}
	reopened, err := Open(config)
	if err != nil {
		t.Fatalf("failed Close retains the lock: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorityRefusesUnsupportedPhaseWithoutMutation(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()
	state := authorityTestState()
	state.Phase = 4
	if _, err := journal.Commit(state); err == nil {
		t.Fatal("Commit accepts an unsupported phase")
	}
	got, err := journal.Snapshot()
	if err != nil || got.Generation != 0 {
		t.Fatalf("rejected phase changes authority: %#v, %v", got, err)
	}
}

func TestAuthorityRefusesCommitAfterUncertainWrite(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()
	fault := errors.New("unavailable authority storage")
	installDurabilityFault(journal, "write-record-body", fault)
	if _, err := journal.Commit(authorityTestState()); !errors.Is(err, fault) {
		t.Fatalf("Commit error = %v", err)
	}
	journal.performIO = nil
	if _, err := journal.Commit(authorityTestState()); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("uncertain authority accepts another commit: %v", err)
	}
}

func TestAuthorityCloseReportsLostDescriptorsAndReleasesLock(t *testing.T) {
	for _, descriptor := range []string{"journal", "lock"} {
		t.Run(descriptor, func(t *testing.T) {
			config := authorityTestConfig(t)
			if err := Initialize(config); err != nil {
				t.Fatal(err)
			}
			journal, err := Open(config)
			if err != nil {
				t.Fatal(err)
			}
			file := journal.journal
			if descriptor == "lock" {
				file = journal.lock
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := journal.Close(); err == nil {
				t.Fatal("Close hides a lost authority descriptor")
			}
			if err := journal.Close(); err != nil {
				t.Fatalf("repeated Close: %v", err)
			}
			reopened, err := Open(config)
			if err != nil {
				t.Fatalf("Close retains the authority lock: %v", err)
			}
			if err := reopened.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpenRefusesUnavailableCheckpointDirectoryAndReleasesLock(t *testing.T) {
	for _, kind := range []string{"file", "public"} {
		t.Run(kind, func(t *testing.T) {
			config := authorityTestConfig(t)
			if err := Initialize(config); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(config.CheckpointDirectory); err != nil {
				t.Fatal(err)
			}
			if kind == "file" {
				if err := os.WriteFile(config.CheckpointDirectory, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(config.CheckpointDirectory, 0o755); err != nil { // #nosec G301 -- The test requires a public checkpoint directory.
				t.Fatal(err)
			}
			journal, err := Open(config)
			if journal != nil {
				_ = journal.Close()
			}
			if err == nil {
				t.Fatal("Open accepts an unavailable private checkpoint directory")
			}
			if err := os.Remove(config.CheckpointDirectory); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(config)
			if err != nil {
				t.Fatalf("failed Open retains the authority lock: %v", err)
			}
			if err := reopened.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpenRefusesOversizedVolumeIdentity(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.Directory, authorityMarkerName), make([]byte, 513), 0o600); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if journal != nil {
		_ = journal.Close()
	}
	if !errors.Is(err, ErrRecordLimit) {
		t.Fatalf("Open error = %v, want bounded identity read", err)
	}
}

type failingAuthorityWriter struct{ err error }

func (w failingAuthorityWriter) Write([]byte) (int, error) { return 0, w.err }

func TestAuthorityWritePreservesFailureAndRejectsNoProgress(t *testing.T) {
	fault := errors.New("authority write failure")
	if err := writeAll(failingAuthorityWriter{err: fault}, []byte("record")); !errors.Is(err, fault) {
		t.Fatalf("write error = %v", err)
	}
	if err := writeAll(failingAuthorityWriter{}, []byte("record")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("zero-progress write error = %v", err)
	}
}
