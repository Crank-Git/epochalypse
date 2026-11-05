package backup

import "golang.org/x/sys/unix"

func renameExclusive(fd int, source, destination string) error {
	return unix.Renameat2(fd, source, fd, destination, unix.RENAME_NOREPLACE)
}
