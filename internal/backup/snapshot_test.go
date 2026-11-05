package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func snapshotSource(t *testing.T) Source {
	t.Helper()
	source := Source{Root: t.TempDir(), Commit: strings.Repeat("a", 40), MaxObjects: 100, QuotaBytes: 1 << 30}
	for _, path := range paths {
		name := filepath.Join(source.Root, path)
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(path, ".db") {
			db := openSnapshotDB(t, name)
			if err := db.Update(func(tx *bolt.Tx) error {
				bucket, err := tx.CreateBucket([]byte("synthetic-v1"))
				if err != nil {
					return err
				}
				return bucket.Put([]byte("record"), []byte(path))
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(name, []byte("synthetic "+path), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return source
}

func openSnapshotDB(t *testing.T, path string) *bolt.DB {
	t.Helper()
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSnapshotPreservesBothCompleteDatabases(t *testing.T) {
	source := snapshotSource(t)
	a, err := Snapshot(context.Background(), source, testLimits(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	if a.Commit != source.Commit || a.CreatedAt.IsZero() || len(a.Files) != len(paths) {
		t.Fatal("snapshot loses provenance or files")
	}
	for _, file := range a.Files {
		original, err := os.ReadFile(filepath.Join(source.Root, file.Path))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, readContent(t, file)) {
			t.Fatal("snapshot changes complete file")
		}
		if file.UID != os.Getuid() || file.GID != os.Getgid() {
			t.Fatal("snapshot loses numeric ownership")
		}
		info, err := os.Stat(file.SourcePath)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("snapshot exposes private file")
		}
		if err := os.WriteFile(filepath.Join(source.Root, file.Path), []byte("changed source"), 0600); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, readContent(t, file)) {
			t.Fatal("source mutation changes snapshot")
		}
	}
	stage := a.stage
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("snapshot close retains plaintext")
	}
}

func TestSnapshotRejectsEitherActiveDatabase(t *testing.T) {
	for _, path := range paths[:2] {
		t.Run(path, func(t *testing.T) {
			source := snapshotSource(t)
			limits := testLimits(t)
			db := openSnapshotDB(t, filepath.Join(source.Root, path))
			defer func() { _ = db.Close() }()
			if a, err := Snapshot(context.Background(), source, limits); err == nil || len(a.Files) != 0 {
				t.Fatal("active database permits snapshot")
			}
			assertEmpty(t, limits.TempDir)
		})
	}
}

func TestSnapshotRejectsUnsafeSourcesAndRemovesPartialFiles(t *testing.T) {
	for _, name := range []string{"missing", "link", "directory", "permissions", "corrupt database", "bound", "cancel", "metadata", "stage"} {
		t.Run(name, func(t *testing.T) {
			source := snapshotSource(t)
			limits := testLimits(t)
			ctx := context.Background()
			if name == "bound" {
				limits.MaxBytes = 1024
			}
			if name == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if name == "metadata" {
				source.Commit = "invalid"
			}
			if name == "stage" {
				limits.TempDir = filepath.Join(limits.TempDir, "absent")
			}
			mutateSnapshotSource(t, source, name)
			if a, err := Snapshot(ctx, source, limits); err == nil || len(a.Files) != 0 {
				t.Fatal("unsafe source permits snapshot")
			}
			if name != "stage" {
				assertEmpty(t, limits.TempDir)
			}
		})
	}
}

func mutateSnapshotSource(t *testing.T, source Source, name string) {
	t.Helper()
	path := filepath.Join(source.Root, paths[len(paths)-1])
	switch name {
	case "missing":
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	case "link":
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(source.Root, paths[2]), path); err != nil {
			t.Fatal(err)
		}
	case "directory":
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	case "permissions":
		if err := os.Chmod(path, 0644); err != nil { // #nosec G302 -- The test rejects public access.
			t.Fatal(err)
		}
	case "corrupt database":
		if err := os.WriteFile(filepath.Join(source.Root, paths[1]), []byte("invalid database"), 0600); err != nil {
			t.Fatal(err)
		}
	}

}

func TestSnapshotHoldsBothExclusiveLocksAndReleasesThem(t *testing.T) {
	source := snapshotSource(t)
	root, err := os.OpenRoot(source.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	dbs, err := lockDatabases(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths[:2] {
		db, err := bolt.Open(filepath.Join(source.Root, path), 0600, &bolt.Options{Timeout: 50 * time.Millisecond})
		if err == nil {
			_ = db.Close()
			t.Fatal("snapshot permits a concurrent database writer")
		}
	}
	closeDatabases(dbs)
	for _, path := range paths[:2] {
		db := openSnapshotDB(t, filepath.Join(source.Root, path))
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSnapshotRejectsInvalidRootsAndLimits(t *testing.T) {
	source := snapshotSource(t)
	limits := testLimits(t)
	limits.MaxBytes = 0
	if _, err := Snapshot(context.Background(), source, limits); err == nil {
		t.Fatal("zero byte limit permits snapshot")
	}
	limits.MaxBytes = DefaultMaxBytes
	source.Root = filepath.Join(source.Root, "absent")
	if _, err := Snapshot(context.Background(), source, limits); err == nil {
		t.Fatal("absent root permits snapshot")
	}
	assertEmpty(t, limits.TempDir)
}

func TestSnapshotRejectsReplacedDatabasesAndChangedFiles(t *testing.T) {
	source := snapshotSource(t)
	root, err := os.OpenRoot(source.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	dbs, err := lockDatabases(root)
	if err != nil {
		t.Fatal(err)
	}
	defer closeDatabases(dbs)
	path := filepath.Join(source.Root, paths[0])
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotFile(context.Background(), root, paths[0], t.TempDir(), DefaultMaxBytes, dbs[0].info); err == nil {
		t.Fatal("replacement database bypasses the held lock")
	}
	if file, err := openExisting(root, paths[0], os.O_RDWR, 0600, dbs[0].info); err == nil {
		_ = file.Close()
		t.Fatal("replacement database bypasses identity check")
	}
	file, err := os.Open(path) // #nosec G304 -- The test creates this path inside its temporary source.
	if err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("different size"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := unchangedSource(file, info); err == nil {
		t.Fatal("source mutation permits snapshot")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := unchangedSource(file, info); err == nil {
		t.Fatal("closed source permits verification")
	}
}

func TestSnapshotRejectsCopyCancellationAndMissingStage(t *testing.T) {
	source := snapshotSource(t)
	root, err := os.OpenRoot(source.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := snapshotFile(ctx, root, paths[2], t.TempDir(), DefaultMaxBytes, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("copy loses cancellation: %v", err)
	}
	if _, err := snapshotFile(context.Background(), root, paths[2], filepath.Join(t.TempDir(), "absent"), DefaultMaxBytes, nil); err == nil {
		t.Fatal("absent stage permits copy")
	}
}

func TestSnapshotRemovesPartialCopiesAfterCancellation(t *testing.T) {
	source := snapshotSource(t)
	limits := testLimits(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	triggered := false
	controlled := &boundaryCancellation{Context: ctx, atBoundary: func() {
		files, err := filepath.Glob(filepath.Join(limits.TempDir, "*", "file-*"))
		if err == nil && len(files) > 0 {
			triggered = true
			cancel()
		}
	}}
	if a, err := Snapshot(controlled, source, limits); !errors.Is(err, context.Canceled) || len(a.Files) != 0 {
		t.Fatalf("copy cancellation exposes partial state: %v", err)
	}
	if !triggered || ctx.Err() != context.Canceled {
		t.Fatal("snapshot does not reach the copy boundary")
	}
	assertEmpty(t, limits.TempDir)
	for _, path := range paths[:2] {
		db := openSnapshotDB(t, filepath.Join(source.Root, path))
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSnapshotRejectsLinkedAndPublicDirectories(t *testing.T) {
	for _, name := range []string{"link", "permissions", "missing database"} {
		t.Run(name, func(t *testing.T) {
			source := snapshotSource(t)
			path := filepath.Join(source.Root, "keys")
			switch name {
			case "link":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("keys.old", path); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if err := os.Chmod(path, 0755); err != nil {
					t.Fatal(err)
				} // #nosec G302 -- The test rejects public access.
			case "missing database":
				if err := os.Remove(filepath.Join(source.Root, paths[0])); err != nil {
					t.Fatal(err)
				}
			}
			limits := testLimits(t)
			if _, err := Snapshot(context.Background(), source, limits); err == nil {
				t.Fatal("unsafe directory permits snapshot")
			}
			assertEmpty(t, limits.TempDir)
		})
	}
}
