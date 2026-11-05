package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Crank-Git/epochalypse/internal/owner"
	bolt "go.etcd.io/bbolt"
)

// ValidateOwner checks the authenticated owner database without a state mutation.
// This check does not validate the other installation files or authorize startup.
func (a Archive) ValidateOwner(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := a.ownerFile()
	if err != nil {
		return err
	}
	info, err := os.Lstat(a.stage)
	if err != nil {
		return fmt.Errorf("inspect private owner stage: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 {
		return errors.New("owner validation requires a private directory without links")
	}
	root, err := os.OpenRoot(a.stage)
	if err != nil {
		return fmt.Errorf("open private owner stage: %w", err)
	}
	defer func() { _ = root.Close() }()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return errors.Join(errors.New("owner stage changes during open"), err)
	}
	return readOwner(ctx, root, file)
}

func (a Archive) ownerFile() (File, error) {
	if a.stage == "" {
		return File{}, errors.New("owner validation requires an authenticated private stage")
	}
	for _, file := range a.Files {
		if file.Path == "state/v1/owner.db" && file.SourcePath == filepath.Join(a.stage, file.Path) {
			return file, nil
		}
	}
	return File{}, errors.New("owner validation requires the authenticated owner file")
}

func readOwner(ctx context.Context, root *os.Root, file File) (err error) {
	info, err := inspectSource(root, file.Path)
	if err != nil {
		return err
	}
	if info.Size() != file.Size {
		return errors.New("authenticated owner database size differs")
	}
	db, err := bolt.Open(file.Path, 0600, &bolt.Options{
		ReadOnly: true, Timeout: 100 * time.Millisecond,
		OpenFile: func(name string, flag int, mode os.FileMode) (*os.File, error) {
			src, err := openExisting(root, name, flag, mode, info)
			if err != nil {
				return nil, err
			}
			if err := copyPayload(ctx, io.Discard, src, file); err != nil {
				_ = src.Close()
				return nil, err
			}
			return src, nil
		},
	})
	if err != nil {
		return fmt.Errorf("open authenticated owner database: %w", err)
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	return db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := owner.ValidateSnapshot(tx); err != nil {
			return fmt.Errorf("validate authenticated owner database: %w", err)
		}
		return ctx.Err()
	})
}
