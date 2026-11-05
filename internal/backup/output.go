package backup

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"filippo.io/age"
)

// SealFile publishes a complete encrypted archive at a fresh path.
func SealFile(ctx context.Context, destination string, archive Archive, recipient *age.X25519Recipient, limits Limits) (err error) {
	if destination == "" {
		return errors.New("backup requires a fresh output path")
	}
	root, directory, err := publicationParent(destination)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close(), directory.Close()) }()
	name := ".epochalypse-encrypted-" + rand.Text()
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create encrypted output: %w", err)
	}
	defer func() { _ = file.Close(); err = errors.Join(err, root.RemoveAll(name)) }()
	if err := Seal(ctx, file, archive, recipient, limits); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync encrypted output: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close encrypted output: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := renameExclusive(int(directory.Fd()), name, filepath.Base(destination)); err != nil {
		return fmt.Errorf("publish encrypted output: %w", err)
	}
	return directory.Sync()
}
