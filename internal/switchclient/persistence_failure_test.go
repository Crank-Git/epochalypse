package switchclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestArmRefusesUnprotectedOrConflictingIntentBeforeTransmission(t *testing.T) {
	for _, kind := range []string{"public", "symlink", "directory", "different", "missing-parent"} {
		t.Run(kind, func(t *testing.T) {
			p, key, replicas := armFixture(t)
			path := replicas.bundle + ".intent"
			switch kind {
			case "public", "different":
				if err := os.WriteFile(path, []byte("existing owner intent"), 0o600); err != nil {
					t.Fatal(err)
				}
				if kind == "public" {
					if err := os.Chmod(path, 0o644); err != nil { // #nosec G302 -- This synthetic fixture verifies refusal of public intent files.
						t.Fatal(err)
					}
				}
			case "directory":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, []byte("protected"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "missing-parent":
				replicas.bundle = filepath.Join(t.TempDir(), "absent", "bundle")
			}
			result, err := PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle)
			if err == nil || result.Confirmed || len(result.Prepared) != 0 || replicas.armCalls != 0 {
				t.Fatalf("unsafe intent permits transmission: confirmed=%t prepared=%d sends=%d error=%v", result.Confirmed, len(result.Prepared), replicas.armCalls, err)
			}
		})
	}
}

func TestArmRefusesBundleFromDifferentSignedIntent(t *testing.T) {
	p, key, replicas := armFixture(t)
	if _, err := PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(replicas.bundle)
	if err != nil {
		t.Fatal(err)
	}
	f, err := switchwire.DecodeFrame(p.Prepare.Body)
	if err != nil {
		t.Fatal(err)
	}
	f.State.Deadline++
	p.Prepare, err = switchwire.SignFrame(f, key)
	if err != nil {
		t.Fatal(err)
	}
	// A separate private intent still cannot reuse another bundle's receipts.
	path := filepath.Join(t.TempDir(), "bundle")
	if err := os.WriteFile(path, before, 0o600); err != nil { // #nosec G703 -- The test chooses this synthetic bundle path in its temporary directory.
		t.Fatal(err)
	}
	result, err := PrepareAndArm(context.Background(), p, key, replicas, path)
	if err == nil || result.Confirmed || replicas.armCalls != 3 {
		t.Fatalf("conflicting bundle permits arm: sends=%d error=%v", replicas.armCalls, err)
	}
	after, err := os.ReadFile(path) // #nosec G304 -- The test chooses this synthetic bundle path in its temporary directory.
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected retry changes recovery evidence")
	}
}

func TestArmCancellationPreservesPrivateIntentWithoutTransmission(t *testing.T) {
	p, key, replicas := armFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := PrepareAndArm(ctx, p, key, replicas, replicas.bundle)
	if !errors.Is(err, context.Canceled) || result.Confirmed || len(result.Prepared) != 0 || replicas.armCalls != 0 {
		t.Fatalf("cancelled operation sends: error=%v", err)
	}
	if _, err := os.Stat(replicas.bundle + ".intent"); err != nil {
		t.Fatal("cancelled operation loses durable intent")
	}
}

type unavailableBundleReader struct{ cause error }

func (r unavailableBundleReader) Read([]byte) (int, error) { return 0, r.cause }

func TestReadBundlePreservesReadFailure(t *testing.T) {
	cause := errors.New("synthetic media failure")
	if _, err := ReadBundle(unavailableBundleReader{cause}, ed25519.PublicKey{}); !errors.Is(err, cause) {
		t.Fatalf("bundle loses read failure: %v", err)
	}
}
