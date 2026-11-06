package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializePreservesMissingVolumeCause(t *testing.T) {
	config := authorityTestConfig(t)
	config.Directory = filepath.Join(config.Directory, "missing")

	err := Initialize(config)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Initialize error = %v, want missing-volume cause", err)
	}
}

func TestOpenReleasesLockWhenJournalIsMissing(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(config.Directory, authorityJournalName)); err != nil {
		t.Fatal(err)
	}

	journal, err := Open(config)
	if journal != nil {
		_ = journal.Close()
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open error = %v, want missing-journal cause", err)
	}

	if err := os.WriteFile(filepath.Join(config.Directory, authorityJournalName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	journal, err = Open(config)
	if err != nil {
		t.Fatalf("Open after journal repair: %v", err)
	}
	closeAuthorityTestJournal(t, journal)
}

func TestInitializeReturnsFilesystemFailures(t *testing.T) {
	for _, operation := range []string{
		"initialize-marker-create",
		"initialize-marker-write",
		"initialize-marker-sync",
		"initialize-marker-close",
		"initialize-journal-create",
		"initialize-journal-sync",
		"initialize-journal-close",
		"initialize-volume-sync",
	} {
		t.Run(operation, func(t *testing.T) {
			config := authorityTestConfig(t)
			fault := errors.New("injected filesystem failure")
			config.performIO = func(name string, action func() error) error {
				if name != operation {
					return action()
				}
				if strings.HasSuffix(name, "-close") {
					return errors.Join(action(), fault)
				}
				return fault
			}

			err := Initialize(config)
			if !errors.Is(err, fault) {
				t.Fatalf("Initialize error = %v, want injected cause", err)
			}
			switch operation {
			case "initialize-marker-create", "initialize-marker-write", "initialize-marker-sync", "initialize-marker-close", "initialize-journal-create":
				journal, openErr := Open(config)
				if journal != nil {
					_ = journal.Close()
				}
				if openErr == nil {
					t.Fatal("Open accepts authority without complete initialization evidence")
				}
			}
		})
	}
}

func TestOpenReturnsReplayFilesystemFailuresAndReleasesLock(t *testing.T) {
	for _, operation := range []string{
		"replay-journal-seek-start",
		"replay-journal-read",
		"replay-journal-seek-end",
	} {
		t.Run(operation, func(t *testing.T) {
			config := authorityTestConfig(t)
			if err := Initialize(config); err != nil {
				t.Fatal(err)
			}
			fault := errors.New("injected replay failure")
			config.performIO = func(name string, action func() error) error {
				if name == operation {
					return fault
				}
				return action()
			}

			journal, err := Open(config)
			if journal != nil {
				_ = journal.Close()
			}
			if !errors.Is(err, fault) {
				t.Fatalf("Open error = %v, want injected cause", err)
			}

			config.performIO = nil
			journal, err = Open(config)
			if err != nil {
				t.Fatalf("Open after replay failure: %v", err)
			}
			closeAuthorityTestJournal(t, journal)
		})
	}
}

func TestOpenRejectsPublicLockAndJournalFiles(t *testing.T) {
	for _, name := range []string{authorityLockName, authorityJournalName} {
		t.Run(name, func(t *testing.T) {
			config := authorityTestConfig(t)
			if err := Initialize(config); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(config.Directory, name)
			if name == authorityLockName {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(path, 0o644); err != nil { // #nosec G302 -- The test requires a public authority file.
				t.Fatal(err)
			}

			journal, err := Open(config)
			if journal != nil {
				_ = journal.Close()
			}
			if err == nil {
				t.Fatal("Open accepts a public authority file")
			}
		})
	}
}
