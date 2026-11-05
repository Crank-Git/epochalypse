package agent

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	authorityBeforeCommitMarker  = "before-commit-marker"
	authorityAfterJournalSync    = "after-journal-sync"
	authorityAfterCheckpointSync = "after-checkpoint-sync"
)

func authorityTestConfig(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	authorityDir := filepath.Join(root, "authority")
	checkpointDir := filepath.Join(root, "archive", "checkpoint")
	if err := os.Mkdir(authorityDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(checkpointDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return Config{
		Directory:           authorityDir,
		CheckpointDirectory: checkpointDir,
		ExpectedVolumeID:    "volume-test-01",
	}
}

func authorityTestState() State {
	return State{
		Generation:      1,
		Phase:           1,
		AcceptedRequest: []byte("signed-request-v2"),
		Enrollment:      []byte("pinned-enrollment-v2"),
		Ciphertext:      []byte("authenticated-ciphertext"),
		Manifest:        []byte("canonical-manifest"),
		EncryptedShare:  []byte("recipient-encrypted-share"),
		Receipt:         []byte("original-signed-receipt"),
		ReleaseEnvelope: []byte("committed-release-envelope"),
	}
}

func TestAuthorityRequiresExplicitInitialization(t *testing.T) {
	config := authorityTestConfig(t)
	if _, err := Open(config); err == nil {
		t.Fatal("Open accepts a missing authority journal")
	}
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorityRejectsDifferentPinnedVolume(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	config.ExpectedVolumeID = "volume-test-02"
	if _, err := Open(config); err == nil {
		t.Fatal("Open accepts a different pinned volume identity")
	}
}

func TestAuthorityPersistsCompleteStateBeforeCheckpoint(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	state := authorityTestState()
	if _, err := journal.Commit(state); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(config.CheckpointDirectory); err != nil {
		t.Fatal(err)
	}
	journal, err = Open(config)
	if err != nil {
		t.Fatal(err)
	}
	closeAuthorityTestJournal(t, journal)
	got, err := journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, state) {
		t.Fatalf("replayed state differs: got %#v, want %#v", got, state)
	}
	if _, err := os.Stat(config.CheckpointDirectory); err != nil {
		t.Fatalf("Open did not repair the missing checkpoint: %v", err)
	}
}

func TestAuthorityRepairsAuthenticOlderCheckpoint(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Commit(authorityTestState()); err != nil {
		t.Fatal(err)
	}
	oldCheckpoint, err := readAuthorityCheckpointBytes(t, config.CheckpointDirectory)
	if err != nil {
		t.Fatal(err)
	}
	next := authorityTestState()
	next.Generation = 2
	next.AcceptedRequest = []byte("next-request")
	if _, err := journal.Commit(next); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writeAuthorityCheckpoint(t, config.CheckpointDirectory, oldCheckpoint); err != nil {
		t.Fatal(err)
	}
	journal, err = Open(config)
	if err != nil {
		t.Fatal(err)
	}
	closeAuthorityTestJournal(t, journal)
	got, err := journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, next) {
		t.Fatalf("replayed state = %#v, want %#v", got, next)
	}
}

func TestAuthorityRejectsDivergentAndNewerCheckpoint(t *testing.T) {
	for _, mutate := range []struct {
		name string
		edit func(*checkpoint)
	}{
		{name: "request_identity", edit: func(saved *checkpoint) { saved.State.AcceptedRequest = []byte("other-request") }},
		{name: "enrollment", edit: func(saved *checkpoint) { saved.State.Enrollment = []byte("other-enrollment") }},
		{name: "ciphertext", edit: func(saved *checkpoint) { saved.State.Ciphertext = []byte("other-ciphertext") }},
		{name: "manifest", edit: func(saved *checkpoint) { saved.State.Manifest = []byte("other-manifest") }},
		{name: "encrypted_share", edit: func(saved *checkpoint) { saved.State.EncryptedShare = []byte("other-share") }},
		{name: "receipt", edit: func(saved *checkpoint) { saved.State.Receipt = []byte("other-receipt") }},
		{name: "release_envelope", edit: func(saved *checkpoint) { saved.State.ReleaseEnvelope = []byte("other-release") }},
		{name: "phase", edit: func(saved *checkpoint) { saved.State.Phase++ }},
		{name: "disclosure_latch", edit: func(saved *checkpoint) { saved.State.PossibleDisclosure = true }},
		{name: "unsafe_clock_latch", edit: func(saved *checkpoint) { saved.State.UnsafeClock = true }},
		{name: "newer", edit: func(saved *checkpoint) { saved.Generation++ }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			config := authorityTestConfig(t)
			if err := Initialize(config); err != nil {
				t.Fatal(err)
			}
			journal, err := Open(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := journal.Commit(authorityTestState()); err != nil {
				t.Fatal(err)
			}
			if err := journal.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := readAuthorityCheckpointBytes(t, config.CheckpointDirectory)
			if err != nil {
				t.Fatal(err)
			}
			var saved checkpoint
			if err := decodeStrictJSON(data, &saved); err != nil {
				t.Fatal(err)
			}
			mutate.edit(&saved)
			saved.Checksum = checkpointChecksum(saved)
			data, err = json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			if err := writeAuthorityCheckpoint(t, config.CheckpointDirectory, data); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(config); err == nil {
				t.Fatal("Open accepts a divergent checkpoint")
			}
		})
	}
}

func TestAuthorityRejectsRollbackOfIrreversibleLatches(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	first := authorityTestState()
	if _, err := journal.Commit(first); err != nil {
		t.Fatal(err)
	}
	second := cloneAuthorityState(first)
	second.Generation = 2
	second.PossibleDisclosure = true
	if _, err := journal.Commit(second); err != nil {
		t.Fatal(err)
	}
	third := cloneAuthorityState(second)
	third.Generation = 3
	third.PossibleDisclosure = false
	if _, err := journal.Commit(third); !errors.Is(err, ErrIrreversibleState) {
		t.Fatalf("Commit error = %v, want ErrIrreversibleState", err)
	}
	third = cloneAuthorityState(second)
	third.Generation = 3
	third.UnsafeClock = true
	if _, err := journal.Commit(third); err != nil {
		t.Fatal(err)
	}
	fourth := cloneAuthorityState(third)
	fourth.Generation = 4
	fourth.UnsafeClock = false
	if _, err := journal.Commit(fourth); !errors.Is(err, ErrIrreversibleState) {
		t.Fatalf("Commit error = %v, want ErrIrreversibleState", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthoritySerializesAccessWithFileLock(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	first, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	closeAuthorityTestJournal(t, first)
	second, err := Open(config)
	if err == nil {
		if closeErr := second.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal("Open accepts a second writer for the same authority volume")
	}
}

func TestAuthorityRejectsCheckpointInsideAuthorityVolume(t *testing.T) {
	config := authorityTestConfig(t)
	config.CheckpointDirectory = filepath.Join(config.Directory, "checkpoint")
	if err := Initialize(config); err == nil {
		t.Fatal("Initialize accepts a checkpoint inside the authority volume")
	}
}

func TestAuthorityFilesUseOwnerOnlyModes(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Commit(authorityTestState()); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := filepath.Walk(config.Directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Errorf("authority path %q has permissions %o", path, info.Mode().Perm())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorityCrashRecoveryPreservesCommitOrdering(t *testing.T) {
	for _, point := range []string{authorityBeforeCommitMarker, authorityAfterJournalSync, authorityAfterCheckpointSync} {
		t.Run(point, func(t *testing.T) {
			testAuthorityCrashAt(t, point)
		})
	}
}

func TestAuthorityRejectsPartialJournalTail(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Commit(authorityTestState()); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(config.Directory)
	if err != nil {
		t.Fatal(err)
	}
	file, err := root.OpenFile("authority.journal", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(errors.Join(err, root.Close()))
	}
	if _, err := file.Write([]byte("partial")); err != nil {
		err = errors.Join(err, file.Close(), root.Close())
		t.Fatal(err)
	}
	if err := errors.Join(file.Close(), root.Close()); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(config); err == nil {
		t.Fatal("Open accepts an incomplete journal suffix")
	}
}

func TestAuthorityCrashHelper(t *testing.T) {
	if os.Getenv("EPOCHALYPSE_AUTHORITY_CRASH_CHILD") != "1" {
		return
	}
	config := Config{
		Directory:           os.Getenv("EPOCHALYPSE_AUTHORITY_DIR"),
		CheckpointDirectory: os.Getenv("EPOCHALYPSE_AUTHORITY_CHECKPOINT"),
		ExpectedVolumeID:    "volume-test-01",
	}
	journal, err := Open(config)
	if err != nil {
		os.Exit(81)
	}
	journal.afterStep = func(point string) {
		if point == os.Getenv("EPOCHALYPSE_AUTHORITY_CRASH_POINT") {
			os.Exit(73)
		}
	}
	state, err := journal.Snapshot()
	if err != nil {
		os.Exit(82)
	}
	state.Generation++
	state.AcceptedRequest = []byte("successor-request")
	state.PossibleDisclosure = true
	if _, err := journal.Commit(state); err != nil {
		os.Exit(83)
	}
	os.Exit(84)
}

func testAuthorityCrashAt(t *testing.T, point string) {
	t.Helper()
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	baseline := authorityTestState()
	if _, err := journal.Commit(baseline); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- testExecutable is the current test binary.
	command := exec.Command(testExecutable, "-test.run=^TestAuthorityCrashHelper$")
	command.Env = append(os.Environ(),
		"EPOCHALYPSE_AUTHORITY_CRASH_CHILD=1",
		"EPOCHALYPSE_AUTHORITY_DIR="+config.Directory,
		"EPOCHALYPSE_AUTHORITY_CHECKPOINT="+config.CheckpointDirectory,
		"EPOCHALYPSE_AUTHORITY_CRASH_POINT="+point,
	)
	err = command.Run()
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 73 {
		t.Fatalf("child exit = %v, want crash at %s", err, point)
	}
	journal, err = Open(config)
	if point == authorityBeforeCommitMarker {
		if err == nil {
			closeAuthorityTestJournal(t, journal)
			t.Fatalf("Open after crash at %s accepts an incomplete record", point)
		}
		return
	}
	if err != nil {
		t.Fatalf("Open after child crash: %v", err)
	}
	closeAuthorityTestJournal(t, journal)
	got, err := journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	want := baseline
	want.Generation++
	want.AcceptedRequest = []byte("successor-request")
	want.PossibleDisclosure = true
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("state after %s = %#v, want %#v", point, got, want)
	}
}

func TestAuthorityRejectsRecordLargerThanConfiguredLimit(t *testing.T) {
	config := authorityTestConfig(t)
	config.MaxRecordBytes = 1024
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	closeAuthorityTestJournal(t, journal)
	state := authorityTestState()
	state.Ciphertext = make([]byte, 2048)
	if _, err := journal.Commit(state); err == nil || !strings.Contains(err.Error(), "record") {
		t.Fatalf("Commit error = %v, want bounded record error", err)
	}
}

func TestAuthorityCheckpointFailureKeepsCommittedJournalState(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(config.CheckpointDirectory, config.CheckpointDirectory+"-saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.CheckpointDirectory, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := authorityTestState()
	if _, err := journal.Commit(want); err == nil {
		t.Fatal("Commit succeeded without a checkpoint directory")
	}
	if _, err := journal.Snapshot(); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("snapshot after checkpoint failure: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(config.CheckpointDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(config.CheckpointDirectory+"-saved", config.CheckpointDirectory); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAuthorityTestJournal(t, restarted)
	got, err := restarted.Snapshot()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("restarted state = %#v, error = %v", got, err)
	}
}

func TestAuthorityRejectsGenerationGap(t *testing.T) {
	config := authorityTestConfig(t)
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	closeAuthorityTestJournal(t, journal)
	state := authorityTestState()
	state.Generation = 2
	if _, err := journal.Commit(state); err == nil || !strings.Contains(err.Error(), "generation") {
		t.Fatalf("Commit error = %v, want generation error", err)
	}
}

func closeAuthorityTestJournal(t *testing.T, journal *Journal) {
	t.Helper()
	t.Cleanup(func() {
		if err := journal.Close(); err != nil {
			t.Errorf("close authority journal: %v", err)
		}
	})
}

func readAuthorityCheckpointBytes(t *testing.T, directory string) ([]byte, error) {
	t.Helper()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open("checkpoint.dat")
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

func writeAuthorityCheckpoint(t *testing.T, directory string, data []byte) error {
	t.Helper()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	file, err := root.OpenFile("checkpoint.dat", os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}
