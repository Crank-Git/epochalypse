package switchclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestReadBundleRejectsUnknownFields(t *testing.T) {
	plan, key, replicas := armFixture(t)
	if _, err := PrepareAndArm(context.Background(), plan, key, replicas, replicas.bundle); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(replicas.bundle)
	if err != nil {
		t.Fatal(err)
	}
	malformed := append(bytes.TrimSuffix(data, []byte("}")), []byte(`,"unexpected":true}`)...)
	owner := key.Public().(ed25519.PublicKey)
	if _, err := ReadBundle(bytes.NewReader(malformed), owner); err == nil || err.Error() != "invalid recovery bundle encoding" {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestPersistExactRejectsOversizedIntent(t *testing.T) {
	data := make([]byte, maxBundleBytes+1)
	if err := persistExact(filepath.Join(t.TempDir(), "intent"), data); err == nil || err.Error() != "owner intent exceeds limit" {
		t.Fatalf("oversized intent error = %v", err)
	}
}

func TestVerifyExactFileRejectsMissingAndNonRegularPaths(t *testing.T) {
	directory := t.TempDir()
	missing := filepath.Join(directory, "missing")
	if err := verifyExactFile(missing, nil); !os.IsNotExist(err) {
		t.Fatalf("missing file error = %v", err)
	}
	if err := verifyExactFile(directory, nil); err == nil || err.Error() != "owner intent must be an owner-only regular file" {
		t.Fatalf("directory error = %v", err)
	}
}

func TestResumePreparedHandlesMissingAndCorruptBundles(t *testing.T) {
	plan, key, _ := armFixture(t)
	prepared, err := resumePrepared(filepath.Join(t.TempDir(), "missing"), plan, key.Public().(ed25519.PublicKey))
	if err != nil || prepared != nil {
		t.Fatalf("missing bundle result = %d receipts, %v", len(prepared), err)
	}
	corrupt := filepath.Join(t.TempDir(), "corrupt")
	if err := os.WriteFile(corrupt, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resumePrepared(corrupt, plan, key.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("corrupt bundle was accepted")
	}
}
