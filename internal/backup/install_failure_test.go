package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestInstallRejectsChangedAuthenticatedFiles(t *testing.T) {
	for _, name := range []string{"canonical path", "size", "digest", "mode", "missing file"} {
		t.Run(name, func(t *testing.T) {
			a := installationArchive(t, nil)
			file := &a.Files[2]
			switch name {
			case "canonical path":
				file.SourcePath = a.Files[3].SourcePath
			case "size":
				file.Size++
			case "digest":
				file.SHA256 = strings.Repeat("0", 64)
			case "mode":
				if err := os.Chmod(file.SourcePath, 0644); err != nil {
					t.Fatal(err)
				} // #nosec G302 -- The rejection test requires an unsafe mode.
			case "missing file":
				if err := os.Remove(file.SourcePath); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.Install(context.Background(), restoreDestination(t, a), testLimits(t)); err == nil {
				t.Fatal("restore accepts changed authenticated files")
			}
		})
	}
}

func TestInstallRejectsInvalidArguments(t *testing.T) {
	a := installationArchive(t, nil)
	for _, name := range []string{"path", "uid", "gid", "reserved uid", "reserved gid", "file owner", "manifest", "byte limit"} {
		t.Run(name, func(t *testing.T) {
			copy := a
			copy.Files = append([]File(nil), a.Files...)
			destination := restoreDestination(t, a)
			limits := testLimits(t)
			switch name {
			case "path":
				destination.Path = ""
			case "uid":
				destination.UID = -1
			case "gid":
				destination.GID = -1
			case "reserved uid":
				destination.UID = 1<<32 - 1
			case "reserved gid":
				destination.GID = 1<<32 - 1
			case "file owner":
				copy.Files[2].UID = 1<<32 - 1
			case "manifest":
				copy.Version = 2
			case "byte limit":
				limits.MaxBytes = 1
			}
			if err := copy.Install(context.Background(), destination, limits); err == nil {
				t.Fatal("restore accepts invalid arguments")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Install(ctx, restoreDestination(t, a), testLimits(t)); !errors.Is(err, context.Canceled) {
		t.Fatal("restore ignores cancellation")
	}
}

func TestInstallRejectsIncompatibleDomainState(t *testing.T) {
	for _, name := range []string{"layout", "window", "capsule", "countdown", "binding", "empty binding"} {
		t.Run(name, func(t *testing.T) {
			a := installationArchive(t, func(root string) {
				db := openSnapshotDB(t, filepath.Join(root, "state/v1/windows.db"))
				if err := db.Update(func(tx *bolt.Tx) error {
					switch name {
					case "layout":
						_, err := tx.CreateBucket([]byte("unsupported-secret-bucket"))
						return err
					case "binding":
						return tx.Bucket([]byte("configuration-v1")).Put([]byte("dnskey"), []byte("wrong key"))
					case "empty binding":
						return tx.Bucket([]byte("configuration-v1")).Delete([]byte("dnskey"))
					default:
						bucket := map[string]string{"window": "windows-v1", "capsule": "capsules-v1", "countdown": "countdowns-v1"}[name]
						return tx.Bucket([]byte(bucket)).Put([]byte("synthetic"), []byte(`{"version":1}`))
					}
				}); err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			})
			if err := a.Install(context.Background(), restoreDestination(t, a), testLimits(t)); err == nil {
				t.Fatal("restore accepts incompatible domain state")
			} else if strings.Contains(err.Error(), "unsupported-secret-bucket") {
				t.Fatal("restore exposes stored identifiers")
			}
		})
	}
}

func TestInstallRejectsInvalidSettingsAndCountdownKey(t *testing.T) {
	for _, name := range []string{"settings", "key"} {
		a := installationArchive(t, func(root string) {
			if name == "key" {
				writeInstallationFile(t, root, "keys/countdown.private", []byte("invalid"))
				return
			}
			writeInstallationFile(t, root, "config/main.json", []byte(`{"version":1,"zone":"windows.example.","countdown_zone":"windows.example."}`))
		})
		if err := a.Install(context.Background(), restoreDestination(t, a), testLimits(t)); err == nil {
			t.Fatal("restore accepts invalid configuration")
		}
	}
}

func TestInstallRejectsQuotaBelowRetainedState(t *testing.T) {
	a := installationArchive(t, func(root string) {
		db := openSnapshotDB(t, filepath.Join(root, "state/v1/windows.db"))
		if err := db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket([]byte("windows-v1")).Put([]byte("synthetic"), []byte(`{"version":1}`))
		}); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})
	a.QuotaBytes = 1
	if err := a.Install(context.Background(), restoreDestination(t, a), testLimits(t)); err == nil {
		t.Fatal("restore accepts state above the archived quota")
	}
}

func TestInstallCancelsBeforePublication(t *testing.T) {
	a := installationArchive(t, nil)
	for _, boundary := range []string{"read", "prepared", "existing destination"} {
		t.Run(boundary, func(t *testing.T) {
			destination := restoreDestination(t, a)
			parent := filepath.Dir(destination.Path)
			var before os.FileInfo
			if boundary == "existing destination" {
				before = existingCancellationDestination(t, destination.Path)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			triggered := false
			controlled := &boundaryCancellation{Context: ctx, atBoundary: func() {
				stages, err := filepath.Glob(filepath.Join(parent, ".epochalypse-install-*"))
				if err != nil || len(stages) != 1 {
					return
				}
				if boundary != "read" && !preparedInstallation(t, a, stages[0]) {
					return
				}
				triggered = true
				cancel()
			}}
			if err := a.Install(controlled, destination, testLimits(t)); !errors.Is(err, context.Canceled) {
				t.Fatalf("restore ignores cancellation at %s: %v", boundary, err)
			}
			if !triggered || ctx.Err() != context.Canceled {
				t.Fatal("restore does not reach the cancellation boundary")
			}
			<-ctx.Done()
			assertCancelledDestination(t, destination.Path, before)
		})
	}
}

func existingCancellationDestination(t *testing.T, path string) os.FileInfo {
	t.Helper()
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func assertCancelledDestination(t *testing.T, path string, before os.FileInfo) {
	t.Helper()
	want := 0
	if before != nil {
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(before, after) {
			t.Fatal("cancelled restore replaces the destination")
		}
		assertEmpty(t, path)
		want = 1
	} else if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("cancelled restore publishes state")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != want {
		t.Fatal("cancelled restore leaves partial state")
	}
}

func preparedInstallation(t *testing.T, a Archive, stage string) bool {
	t.Helper()
	// Complete bytes identify prepared state without depending on read chunk sizes.
	for _, file := range a.Files {
		data, err := os.ReadFile(filepath.Join(stage, file.Path)) // #nosec G304 -- The fixture selects the private stage and archive paths.
		if err != nil || !bytes.Equal(data, readContent(t, file)) {
			return false
		}
	}
	return true
}

func TestInstallUsesReadOnlyInstallationAccess(t *testing.T) {
	a := installationArchive(t, nil)
	file := &a.Files[1]
	// A read-only holder proves that validation does not obtain an exclusive writer lock.
	db, err := bolt.Open(file.SourcePath, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := a.Install(context.Background(), restoreDestination(t, a), testLimits(t)); err != nil {
		t.Fatal(err)
	}
}
