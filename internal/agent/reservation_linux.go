//go:build linux

package agent

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func reservePhysicalFile(file *os.File, size int64) error {
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(int(file.Fd()), &filesystem); err != nil {
		return err
	}
	// Copy-on-write filesystems need a separate overwrite-reservation proof.
	if filesystem.Type != unix.EXT4_SUPER_MAGIC {
		return errors.New("physical reservation requires ext4")
	}
	var before unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &before); err != nil {
		return err
	}
	if before.Size > size {
		return ErrCapacity
	}
	if before.Blocks*512 < size {
		if err := unix.Fallocate(int(file.Fd()), unix.FALLOC_FL_KEEP_SIZE, 0, size); err != nil {
			return fmt.Errorf("allocate physical capacity: %w", err)
		}
	}
	var after unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &after); err != nil {
		return err
	}
	if after.Size != before.Size || after.Blocks*512 < size {
		return errors.New("filesystem does not preserve reserved capacity and logical size")
	}
	return file.Sync()
}

func exchangePhysicalCheckpoints(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	err = unix.Renameat2(int(directory.Fd()), authorityCheckpointName, int(directory.Fd()), checkpointSpareName, unix.RENAME_EXCHANGE)
	return errors.Join(err, directory.Close())
}

func inspectPhysicalReservation(file *os.File, size int64) error {
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(int(file.Fd()), &filesystem); err != nil {
		return err
	}
	if filesystem.Type != unix.EXT4_SUPER_MAGIC {
		return errors.New("physical reservation requires ext4")
	}
	var info unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &info); err != nil {
		return err
	}
	if info.Size > size || info.Blocks*512 < size {
		return ErrCapacity
	}
	return nil
}
