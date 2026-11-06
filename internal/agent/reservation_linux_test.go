//go:build linux

package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPhysicalReservationPreservesBlocksAndLogicalJournalAcrossRestart(t *testing.T) {
	config := authorityTestConfig(t)
	config.LabOnlySyntheticStorage = false
	config.MaxRecordBytes = 1024
	config.MaxJournalBytes = 8192
	config.MaxJournalRecords = 6
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	j, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	assertPhysicalReservationBlocks(t, config)
	info, err := j.journal.Stat()
	if err != nil || info.Size() != 0 {
		t.Fatalf("journal logical size changes: %v", err)
	}
	state := authorityTestState()
	for state.Generation <= 4 {
		if _, err := j.Commit(state); err != nil {
			t.Fatal(err)
		}
		state.Generation++
	}
	if _, err := j.Commit(state); !errors.Is(err, ErrCapacity) {
		t.Fatalf("admission capacity error=%v", err)
	}
	state.PossibleDisclosure = true
	if _, err := j.Commit(state); err != nil {
		t.Fatalf("reserved release fails: %v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	state, err = j.Snapshot()
	if err != nil || state.Generation != 5 || !state.PossibleDisclosure {
		t.Fatalf("restart loses authority: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.CheckpointDirectory, checkpointSpareName)); err != nil {
		t.Fatal(err)
	}
}

func TestPhysicalReservationReadinessRejectsLostBlocksWithoutRepair(t *testing.T) {
	config := authorityTestConfig(t)
	config.LabOnlySyntheticStorage = false
	config.MaxRecordBytes = 1024
	config.MaxJournalBytes = 8192
	config.MaxJournalRecords = 6
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()
	if !journal.readinessStorage() {
		t.Fatal("reserved storage fails readiness")
	}
	if err := unix.Fallocate(int(journal.journal.Fd()), unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE, 0, config.MaxJournalBytes); err != nil {
		t.Fatal(err)
	}
	if journal.readinessStorage() {
		t.Fatal("lost physical reservation passes readiness")
	}
	var stat unix.Stat_t
	if err := unix.Fstat(int(journal.journal.Fd()), &stat); err != nil {
		t.Fatal(err)
	}
	if stat.Blocks != 0 {
		t.Fatal("readiness repairs lost physical reservation")
	}
}

func assertPhysicalReservationBlocks(t *testing.T, config Config) {
	t.Helper()
	for _, item := range []struct {
		directory, name string
		minimum         int64
	}{
		{config.Directory, authorityJournalName, config.MaxJournalBytes},
		{config.CheckpointDirectory, authorityCheckpointName, int64(config.MaxRecordBytes)},
		{config.CheckpointDirectory, checkpointSpareName, int64(config.MaxRecordBytes)},
	} {
		var stat unix.Stat_t
		if err := unix.Stat(filepath.Join(item.directory, item.name), &stat); err != nil {
			t.Fatal(err)
		}
		if stat.Blocks*512 < item.minimum {
			t.Fatalf("%s allocated %d bytes", item.name, stat.Blocks*512)
		}
	}
}
