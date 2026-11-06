package switchclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestExportBundleRejectsUnsafeExistingPaths(t *testing.T) {
	p, key, replicas := armFixture(t)
	if _, err := PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle); err != nil {
		t.Fatal(err)
	}
	owner := key.Public().(ed25519.PublicKey)
	file, err := os.Open(replicas.bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := ReadBundle(file, owner)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	for _, kind := range []string{"directory", "symlink", "public"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "export")
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "public":
				if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o644); err != nil { // #nosec G302 -- The fixture verifies rejection of a public recovery file.
					t.Fatal(err)
				}
			}
			if err := ExportBundle(path, bundle, owner); err == nil {
				t.Fatal("unsafe existing export path accepted")
			}
		})
	}
}

func TestReadBundleRejectsInputAboveLimit(t *testing.T) {
	data := append([]byte(`{"Version":1}`), bytes.Repeat([]byte(" "), maxBundleBytes)...)
	if _, err := ReadBundle(bytes.NewReader(data), ed25519.PublicKey{}); err == nil || err.Error() != "recovery bundle exceeds limit" {
		t.Fatalf("oversized bundle error = %v", err)
	}
}

func TestArmStopsWhenArmEvidenceConflicts(t *testing.T) {
	p, key, replicas := armFixture(t)
	armEvidence := replicas.bundle + ".arm"
	if err := os.WriteFile(armEvidence, []byte("different owner intent"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle)
	if err == nil || result.Confirmed || replicas.armCalls != 0 || replicas.prepareCalls != 3 {
		t.Fatalf("conflicting arm evidence permits transmission: result=%+v prepares=%d sends=%d error=%v", result, replicas.prepareCalls, replicas.armCalls, err)
	}
}
