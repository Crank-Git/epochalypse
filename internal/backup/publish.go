package backup

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (a Archive) publish(ctx context.Context, destination string, uid, gid int) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validIdentity(uid, gid) {
		return errors.New("restore requires a numeric service identity")
	}
	root, directory, err := publicationParent(destination)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close(), directory.Close()) }()
	name := ".epochalypse-install-" + rand.Text()
	if err := root.Mkdir(name, 0700); err != nil {
		return fmt.Errorf("create restore stage: %w", err)
	}
	defer func() { err = errors.Join(err, root.RemoveAll(name)) }()
	stage, err := root.OpenRoot(name)
	if err != nil {
		return fmt.Errorf("open restore stage: %w", err)
	}
	defer func() { err = errors.Join(err, stage.Close()) }()
	for _, file := range a.Files {
		if err := stage.MkdirAll(filepath.Dir(file.Path), 0700); err != nil {
			return fmt.Errorf("create restored directory: %w", err)
		}
		if err := installFile(ctx, stage, file, uid, gid); err != nil {
			return err
		}
	}
	if err := syncDirectories(stage, uid, gid); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Exclusive rename preserves an empty destination created by a concurrent operator.
	if err := renameExclusive(int(directory.Fd()), name, filepath.Base(destination)); err != nil {
		return fmt.Errorf("publish fresh restore: %w", err)
	}
	return directory.Sync()
}

func publicationParent(destination string) (*os.Root, *os.File, error) {
	if err := regularParent(destination); err != nil {
		return nil, nil, err
	}
	parent := filepath.Dir(destination)
	root, err := os.OpenRoot(parent)
	if err != nil {
		return nil, nil, fmt.Errorf("open restore parent: %w", err)
	}
	directory, err := os.Open(parent) // #nosec G304 -- The operator selects the restore parent.
	if err != nil {
		_ = root.Close()
		return nil, nil, fmt.Errorf("open restore directory: %w", err)
	}
	info, err := root.Stat(".")
	opened, statErr := directory.Stat()
	if err != nil || statErr != nil || !os.SameFile(info, opened) {
		_ = root.Close()
		_ = directory.Close()
		return nil, nil, errors.Join(errors.New("restore parent changes during open"), err, statErr)
	}
	return root, directory, nil
}

func installFile(ctx context.Context, root *os.Root, file File, uid, gid int) (err error) {
	dst, err := root.OpenFile(file.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create restored file: %w", err)
	}
	defer func() { err = errors.Join(err, dst.Close()) }()
	if err := copySource(ctx, dst, file); err != nil {
		return err
	}
	if err := dst.Chown(uid, gid); err != nil {
		return fmt.Errorf("set restored file identity: %w", err)
	}
	return dst.Sync()
}

func syncDirectories(root *os.Root, uid, gid int) error {
	for _, path := range []string{"state/v1", "state", "config", "keys", "tls", "."} {
		dir, err := root.Open(path)
		if err != nil {
			return fmt.Errorf("open restored directory: %w", err)
		}
		err = errors.Join(dir.Chown(uid, gid), dir.Sync(), dir.Close())
		if err != nil {
			return fmt.Errorf("sync restored directory: %w", err)
		}
	}
	return nil
}
