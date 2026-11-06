package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const checkpointSpareName = "checkpoint.spare"

func (j *Journal) checkCapacity(next State, payloadBytes int) error {
	saved := checkpoint{Version: authorityFormatVersion, Generation: next.Generation, HeadSHA256: hex.EncodeToString(j.headHash[:]), State: next}
	saved.Checksum = checkpointChecksum(saved)
	data, _ := json.Marshal(saved)
	if len(data) > int(j.config.MaxRecordBytes) {
		return ErrRecordLimit
	}
	info, err := j.journal.Stat()
	if err != nil {
		return fmt.Errorf("inspect authority capacity: %w", err)
	}
	remaining := reservedRecords(next)
	frameBytes := int64(payloadBytes + 8 + sha256.Size + len(authorityCommitMarker))
	if next.Generation > j.config.MaxJournalRecords-uint64(remaining) || info.Size()+frameBytes > j.config.MaxJournalBytes-int64(remaining)*(int64(j.config.MaxRecordBytes)+48) {
		return ErrCapacity
	}
	return nil
}

func reservedRecords(next State) uint8 {
	var remaining uint8
	if !next.PossibleDisclosure {
		remaining++
	}
	if !next.UnsafeClock {
		remaining++
	}
	// An accepted intent also needs its durable receipt before the caller sees success.
	if next.Phase != 0 && len(next.Receipt) == 0 && !next.PossibleDisclosure && !next.UnsafeClock {
		remaining++
	}
	return remaining
}

func (j *Journal) createCheckpointDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	// Repeat parent sync after restart because a previous creation may have failed there.
	for current := path; ; current = filepath.Dir(current) {
		if err := j.io("sync-checkpoint-parent", func() error {
			directory, err := os.Open(current) // #nosec G304 -- The validated local checkpoint path fixes this parent hierarchy.
			if err != nil {
				return err
			}
			return errors.Join(directory.Sync(), directory.Close())
		}); err != nil {
			return err
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}

func (j *Journal) prepareStorage() error {
	if j.storageReady {
		return nil
	}
	if err := j.io("reserve-journal", func() error { return j.reserveFile(j.journal, j.config.MaxJournalBytes) }); err != nil {
		return fmt.Errorf("reserve authority journal: %w", err)
	}
	root, err := os.OpenRoot(j.config.CheckpointDirectory)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	for _, name := range []string{authorityCheckpointName, checkpointSpareName} {
		if err := j.reserveCheckpoint(root, name); err != nil {
			return err
		}
	}
	active, err := root.Stat(authorityCheckpointName)
	if err != nil {
		return err
	}
	spare, err := root.Stat(checkpointSpareName)
	if err != nil {
		return err
	}
	if os.SameFile(active, spare) {
		return fmt.Errorf("%w: checkpoint slots share an inode", ErrQuarantined)
	}
	if err := syncRoot(root); err != nil {
		return err
	}
	j.storageReady = true
	return nil
}

func (j *Journal) reserveFile(file *os.File, size int64) error {
	if j.config.LabOnlySyntheticStorage {
		return nil
	}
	return reservePhysicalFile(file, size)
}

func (j *Journal) reserveCheckpoint(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && (!ownerOnlyFile(info) || info.Size() > int64(j.config.MaxRecordBytes)) {
		return fmt.Errorf("%w: checkpoint slot is not a private regular file", ErrQuarantined)
	}
	file, err := root.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open checkpoint reservation: %w", err)
	}
	if err := j.io("reserve-checkpoint", func() error { return j.reserveFile(file, int64(j.config.MaxRecordBytes)) }); err != nil {
		return errors.Join(err, file.Close())
	}
	return errors.Join(file.Sync(), file.Close())
}

func (j *Journal) writeCheckpoint() error {
	if err := j.prepareStorage(); err != nil {
		return err
	}
	saved := checkpoint{Version: authorityFormatVersion, Generation: j.state.Generation, HeadSHA256: hex.EncodeToString(j.headHash[:]), State: j.state}
	saved.Checksum = checkpointChecksum(saved)
	// These concrete fields cannot make json.Marshal fail.
	data, _ := json.Marshal(saved)
	if len(data) > int(j.config.MaxRecordBytes) {
		return ErrRecordLimit
	}
	// Whitespace keeps strict JSON valid without releasing reserved extents.
	data = append(data, bytes.Repeat([]byte{' '}, int(j.config.MaxRecordBytes)-len(data))...)
	if err := j.io("create-checkpoint-directory", func() error { return checkPrivateDirectory(j.config.CheckpointDirectory) }); err != nil {
		return err
	}
	var root *os.Root
	err := j.io("open-checkpoint-root", func() error {
		var openErr error
		root, openErr = os.OpenRoot(j.config.CheckpointDirectory)
		return openErr
	})
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := j.writeSpareCheckpoint(root, data); err != nil {
		return err
	}
	if err := j.io("rename-checkpoint", func() error { return exchangeCheckpoints(root, j.config.LabOnlySyntheticStorage) }); err != nil {
		return fmt.Errorf("exchange checkpoint slots: %w", err)
	}
	return j.io("sync-checkpoint-directory", func() error { return syncRoot(root) })
}

func (j *Journal) writeSpareCheckpoint(root *os.Root, data []byte) error {
	var file *os.File
	err := j.io("create-checkpoint-file", func() error {
		var openErr error
		file, openErr = root.OpenFile(checkpointSpareName, os.O_WRONLY, 0)
		return openErr
	})
	if err != nil {
		return err
	}
	if err := j.io("protect-checkpoint-file", func() error {
		info, statErr := file.Stat()
		if statErr != nil {
			return statErr
		}
		if !ownerOnlyFile(info) {
			return ErrQuarantined
		}
		return nil
	}); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := j.io("write-checkpoint", func() error { return writeAll(file, data) }); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := j.io("sync-checkpoint", file.Sync); err != nil {
		return errors.Join(err, file.Close())
	}
	return j.io("close-checkpoint", file.Close)
}

func exchangeCheckpoints(root *os.Root, synthetic bool) error {
	if !synthetic {
		return exchangePhysicalCheckpoints(root)
	}
	// This explicit lab path supplies no atomicity or physical-capacity claim.
	if err := root.Rename(authorityCheckpointName, "checkpoint.lab-swap"); err != nil {
		return err
	}
	if err := root.Rename(checkpointSpareName, authorityCheckpointName); err != nil {
		return err
	}
	return root.Rename("checkpoint.lab-swap", checkpointSpareName)
}
