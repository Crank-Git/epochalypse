package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/owner"
	bolt "go.etcd.io/bbolt"
)

func authenticatedOwner(t *testing.T, change func(*bolt.Tx) error) Archive {
	t.Helper()
	dir := t.TempDir()
	s, err := owner.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize([]byte("synthetic backup password")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "v1", "owner.db")
	if change != nil {
		db := openSnapshotDB(t, path)
		if err := db.Update(change); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	a := fixture(t)
	data, err := os.ReadFile(path) // #nosec G304 -- The fixture creates this database in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	a.Files[0].SourcePath = path
	a.Files[0].Size = int64(len(data))
	a.Files[0].SHA256 = hex.EncodeToString(digest[:])
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var ciphertext bytes.Buffer
	if err := Seal(context.Background(), &ciphertext, a, identity.Recipient(), testLimits(t)); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(context.Background(), &ciphertext, identity, testLimits(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Error(err)
		}
	})
	return opened
}

func TestAuthenticatedOwnerValidationPreservesDatabase(t *testing.T) {
	a := authenticatedOwner(t, nil)
	before := readContent(t, a.Files[0])
	if err := a.ValidateOwner(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, readContent(t, a.Files[0])) {
		t.Fatal("owner validation changes database bytes")
	}
}

func TestAuthenticatedOwnerValidationUsesReadOnlyAccess(t *testing.T) {
	a := authenticatedOwner(t, nil)
	db, err := bolt.Open(a.Files[0].SourcePath, 0600, &bolt.Options{ReadOnly: true, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := a.ValidateOwner(context.Background()); err != nil {
		t.Fatalf("owner validation requires an exclusive write lock: %v", err)
	}
}

func TestAuthenticatedOwnerValidationRejectsUnknownRecords(t *testing.T) {
	a := authenticatedOwner(t, func(tx *bolt.Tx) error {
		_, err := tx.CreateBucket([]byte("secret-unsupported-owner-bucket"))
		return err
	})
	err := a.ValidateOwner(context.Background())
	if err == nil {
		t.Fatal("authenticated archive bypasses owner record validation")
	}
	if strings.Contains(err.Error(), "secret-unsupported-owner-bucket") {
		t.Fatal("owner validation exposes stored data")
	}
}

func TestAuthenticatedOwnerValidationRejectsUnavailableStage(t *testing.T) {
	a := authenticatedOwner(t, nil)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.ValidateOwner(context.Background()); err == nil {
		t.Fatal("owner validation accepts removed state")
	}
	if err := (Archive{}).ValidateOwner(context.Background()); err == nil {
		t.Fatal("owner validation accepts unauthenticated state")
	}
}

func TestAuthenticatedOwnerValidationRejectsCancellation(t *testing.T) {
	a := authenticatedOwner(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.ValidateOwner(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("owner validation ignores cancellation: %v", err)
	}
}

type boundaryCancellation struct {
	context.Context
	atBoundary func()
}

func (c *boundaryCancellation) Err() error {
	c.atBoundary()
	return c.Context.Err()
}

func TestAuthenticatedOwnerPayloadCancelsDuringRead(t *testing.T) {
	a := authenticatedOwner(t, nil)
	src, err := os.Open(a.Files[0].SourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := cancelAfterRead{Reader: io.LimitReader(src, 1), cancel: cancel}
	var copied bytes.Buffer
	if err := copyPayload(ctx, &copied, reader, a.Files[0]); !errors.Is(err, context.Canceled) {
		t.Fatalf("owner payload ignores read cancellation: %v", err)
	}
	if copied.Len() != 1 || ctx.Err() != context.Canceled {
		t.Fatal("owner payload does not cancel after the first byte")
	}
}

type cancelAfterRead struct {
	io.Reader
	cancel context.CancelFunc
}

func (r cancelAfterRead) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.cancel()
	return n, err
}

func TestAuthenticatedOwnerValidationRejectsMissingDatabase(t *testing.T) {
	a := authenticatedOwner(t, nil)
	if err := os.Remove(a.Files[0].SourcePath); err != nil {
		t.Fatal(err)
	}
	if err := a.ValidateOwner(context.Background()); err == nil {
		t.Fatal("owner validation accepts a missing database")
	}
}

func TestAuthenticatedOwnerValidationRejectsReplacedFiles(t *testing.T) {
	for _, name := range []string{"path", "link", "mode", "digest", "missing owner", "size"} {
		t.Run(name, func(t *testing.T) {
			a := authenticatedOwner(t, nil)
			path := a.Files[0].SourcePath
			switch name {
			case "path":
				a.Files[0].SourcePath = fixture(t).Files[0].SourcePath
			case "link":
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".original", path); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(path, 0644); err != nil { // #nosec G302 -- The rejection test requires an unsafe fixture mode.
					t.Fatal(err)
				}
			case "digest":
				a.Files[0].SHA256 = strings.Repeat("0", 64)
			case "missing owner":
				a.Files = a.Files[1:]
			case "size":
				a.Files[0].Size--
			}
			if err := a.ValidateOwner(context.Background()); err == nil {
				t.Fatal("owner validation accepts replaced state")
			}
		})
	}
}

func TestAuthenticatedOwnerValidationRejectsInvalidDatabase(t *testing.T) {
	a := authenticatedOwner(t, nil)
	data := bytes.Repeat([]byte("x"), int(a.Files[0].Size))
	if err := os.WriteFile(a.Files[0].SourcePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	a.Files[0].SHA256 = hex.EncodeToString(digest[:])
	if err := a.ValidateOwner(context.Background()); err == nil {
		t.Fatal("owner validation accepts an invalid database")
	}
}

func TestAuthenticatedOwnerValidationRejectsUnsafeStage(t *testing.T) {
	for _, name := range []string{"mode", "link"} {
		t.Run(name, func(t *testing.T) {
			a := authenticatedOwner(t, nil)
			if name == "mode" {
				if err := os.Chmod(a.stage, 0755); err != nil { // #nosec G302 -- The rejection test requires an unsafe fixture mode.
					t.Fatal(err)
				}
			} else {
				if err := os.Rename(a.stage, a.stage+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(a.stage+".original", a.stage); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.ValidateOwner(context.Background()); err == nil {
				t.Fatal("owner validation accepts an unsafe stage")
			}
		})
	}
}
