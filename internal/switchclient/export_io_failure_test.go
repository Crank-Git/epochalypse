package switchclient

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestArmStopsWhenRecoveryFileOperationFails(t *testing.T) {
	for _, operation := range []string{"write", "sync", "close"} {
		for _, suffix := range []string{".intent", ""} {
			t.Run(operation+suffix, func(t *testing.T) {
				p, key, replicas := armFixture(t)
				cause := errors.New("synthetic recovery file failure")
				installRecoveryFileFailure(t, operation, replicas.bundle+suffix, cause)

				result, err := PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle)
				if !errors.Is(err, cause) || result.Confirmed || replicas.armCalls != 0 {
					t.Fatalf("file failure permits arm: confirmed=%t sends=%d error=%v", result.Confirmed, replicas.armCalls, err)
				}
				wantPrepareCalls := 3
				if suffix == ".intent" {
					wantPrepareCalls = 0
				}
				if replicas.prepareCalls != wantPrepareCalls {
					t.Fatalf("prepare calls = %d, want %d", replicas.prepareCalls, wantPrepareCalls)
				}
			})
		}
	}
}

func TestExportBundleReturnsRecoveryFileFailure(t *testing.T) {
	p, key, replicas := armFixture(t)
	if _, err := PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(replicas.bundle)
	if err != nil {
		t.Fatal(err)
	}
	owner := key.Public().(ed25519.PublicKey)
	bundle, err := ReadBundle(file, owner)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "export")
	cause := errors.New("synthetic export close failure")
	installRecoveryFileFailure(t, "close", path, cause)
	if err := ExportBundle(path, bundle, owner); !errors.Is(err, cause) {
		t.Fatalf("export error = %v, want cause", err)
	}
}

func installRecoveryFileFailure(t *testing.T, operation, path string, cause error) {
	t.Helper()
	oldWrite, oldSync, oldClose := writeBundleFile, syncBundleFile, closeBundleFile
	t.Cleanup(func() {
		writeBundleFile, syncBundleFile, closeBundleFile = oldWrite, oldSync, oldClose
	})
	if operation == "write" {
		writeBundleFile = func(got string, file *os.File, data []byte) error {
			if got == path {
				return cause
			}
			return oldWrite(got, file, data)
		}
	}
	if operation == "sync" {
		syncBundleFile = func(got string, file *os.File) error {
			if got == path {
				return cause
			}
			return oldSync(got, file)
		}
	}
	if operation == "close" {
		closeBundleFile = func(got string, file *os.File) error {
			if got == path {
				return errors.Join(cause, file.Close())
			}
			return oldClose(got, file)
		}
	}
}
