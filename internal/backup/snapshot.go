package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Source identifies a stopped main-service installation with canonical archive paths.
// The operator must stop configuration and key writers until Snapshot returns.
type Source struct {
	Root       string
	Commit     string
	MaxObjects int
	QuotaBytes int64
}

// Snapshot returns private copies while exclusive locks block both database writers.
// The caller must call Close to remove the copies. No source file changes.
func Snapshot(ctx context.Context, source Source, limits Limits) (result Archive, err error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if limits.MaxBytes < 1 {
		return result, errors.New("backup requires a positive byte limit")
	}
	if err := regularParent(filepath.Join(source.Root, "snapshot")); err != nil {
		return result, err
	}
	root, err := os.OpenRoot(source.Root)
	if err != nil {
		return result, fmt.Errorf("open snapshot root: %w", err)
	}
	defer func() { _ = root.Close() }()
	dbs, err := lockDatabases(root)
	if err != nil {
		return result, err
	}
	// Both locks remain held until every database, configuration, and key copy completes.
	defer closeDatabases(dbs)
	stage, err := os.MkdirTemp(limits.TempDir, "epochalypse-snapshot-*")
	if err != nil {
		return result, fmt.Errorf("create private snapshot stage: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(stage)
		}
	}()
	result = Archive{Version: 1, Commit: source.Commit, CreatedAt: time.Now().UTC(), MaxObjects: source.MaxObjects, QuotaBytes: source.QuotaBytes, stage: stage}
	remaining := limits.MaxBytes
	for i, path := range paths {
		var locked os.FileInfo
		if i < len(dbs) {
			locked = dbs[i].info
		}
		file, copyErr := snapshotFile(ctx, root, path, stage, remaining, locked)
		if copyErr != nil {
			return Archive{}, copyErr
		}
		remaining -= file.Size
		result.Files = append(result.Files, file)
	}
	if _, err = result.manifest(limits); err != nil {
		return Archive{}, err
	}
	if err = ctx.Err(); err != nil {
		return Archive{}, err
	}
	return result, nil
}

type lockedDatabase struct {
	db   *bolt.DB
	info os.FileInfo
}

func closeDatabases(dbs []lockedDatabase) {
	for _, db := range dbs {
		_ = db.db.Close()
	}
}

func lockDatabases(root *os.Root) (dbs []lockedDatabase, err error) {
	defer func() {
		if err != nil {
			closeDatabases(dbs)
		}
	}()
	for _, path := range paths[:2] {
		var info os.FileInfo
		if info, err = inspectSource(root, path); err != nil {
			return dbs, err
		}
		var db *bolt.DB
		// O_CREATE would silently create a missing database instead of rejecting the source.
		db, err = bolt.Open(path, 0600, &bolt.Options{Timeout: 100 * time.Millisecond, OpenFile: func(name string, flag int, mode os.FileMode) (*os.File, error) {
			return openExisting(root, name, flag, mode, info)
		}})
		if err != nil {
			return dbs, fmt.Errorf("lock stopped snapshot database: %w", err)
		}
		dbs = append(dbs, lockedDatabase{db: db, info: info})
	}
	return dbs, nil
}

func openExisting(root *os.Root, name string, flag int, mode os.FileMode, before os.FileInfo) (*os.File, error) {
	file, err := root.OpenFile(name, flag&^os.O_CREATE, mode)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, errors.Join(errors.New("snapshot database changes during open"), err)
	}
	return file, nil
}

func inspectSource(root *os.Root, path string) (os.FileInfo, error) {
	for parent := filepath.Dir(path); parent != "."; parent = filepath.Dir(parent) {
		info, err := root.Lstat(parent)
		if err != nil {
			return nil, fmt.Errorf("inspect snapshot directory: %w", err)
		}
		if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("snapshot requires private directories without links")
		}
	}
	info, err := root.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect snapshot source: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() == 0 {
		return nil, errors.New("snapshot requires nonempty mode 0600 regular files")
	}
	return info, nil
}

func snapshotFile(ctx context.Context, root *os.Root, path, stage string, remaining int64, locked os.FileInfo) (File, error) {
	info, err := inspectSource(root, path)
	if err != nil {
		return File{}, err
	}
	if info.Size() > remaining {
		return File{}, errors.New("snapshot exceeds the byte limit")
	}
	if locked != nil && !os.SameFile(info, locked) {
		return File{}, errors.New("snapshot database changes after lock")
	}
	src, err := root.Open(path)
	if err != nil {
		return File{}, fmt.Errorf("open snapshot source: %w", err)
	}
	defer func() { _ = src.Close() }()
	opened, err := src.Stat()
	if err != nil {
		return File{}, fmt.Errorf("inspect open snapshot source: %w", err)
	}
	if !os.SameFile(info, opened) {
		return File{}, errors.New("snapshot source changes during open")
	}
	dst, err := os.CreateTemp(stage, "file-*")
	if err != nil {
		return File{}, fmt.Errorf("create snapshot file: %w", err)
	}
	defer func() { _ = dst.Close() }()
	digest := sha256.New()
	if _, err = io.CopyN(io.MultiWriter(dst, digest), contextReader{ctx, src}, info.Size()); err != nil {
		return File{}, fmt.Errorf("copy snapshot source: %w", err)
	}
	if err := unchangedSource(src, info); err != nil {
		return File{}, err
	}
	if err := dst.Close(); err != nil {
		return File{}, fmt.Errorf("close snapshot file: %w", err)
	}
	owner := info.Sys().(*syscall.Stat_t)
	return File{Path: path, UID: int(owner.Uid), GID: int(owner.Gid), Size: info.Size(), SHA256: hex.EncodeToString(digest.Sum(nil)), SourcePath: dst.Name()}, nil
}

func unchangedSource(src *os.File, before os.FileInfo) error {
	after, err := src.Stat()
	if err != nil {
		return fmt.Errorf("inspect copied snapshot source: %w", err)
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("snapshot source changes during copy")
	}
	return nil
}

func regularParent(path string) error {
	info, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("inspect backup source directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("backup source directory requires no link")
	}
	return nil
}
