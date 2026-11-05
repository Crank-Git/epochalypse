package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func TestSealFilePublishesAuthenticatedArchive(t *testing.T) {
	ctx := context.Background()
	limits := Limits{MaxBytes: DefaultMaxBytes, TempDir: t.TempDir()}
	archive, err := Snapshot(ctx, Source{Root: installationSource(t), Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", MaxObjects: 100, QuotaBytes: 1 << 30}, limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = archive.Close() })
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.age")
	if err := SealFile(ctx, path, archive, identity.Recipient(), limits); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("protected output: %v %v", info, err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- The test creates this private archive path.
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Open(ctx, bytes.NewReader(data), identity, limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	if restored.Commit != archive.Commit {
		t.Fatal("archive commit differs")
	}
	if err := SealFile(ctx, path, archive, identity.Recipient(), limits); err == nil {
		t.Fatal("replaced existing output")
	}
	after, err := os.ReadFile(path) // #nosec G304 -- The test creates this private archive path.
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("existing output changes")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary output remains: %v %v", entries, err)
	}
}

func TestSealFileRemovesFailedOutput(t *testing.T) {
	ctx := context.Background()
	limits := Limits{MaxBytes: DefaultMaxBytes, TempDir: t.TempDir()}
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.age")
	if err := SealFile(ctx, path, Archive{}, nil, limits); err == nil {
		t.Fatal("accepts invalid archive")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed output remains: %v %v", entries, err)
	}
	for _, name := range []string{"", filepath.Join(dir, "missing", "backup.age")} {
		if err := SealFile(ctx, name, Archive{}, nil, limits); err == nil {
			t.Fatalf("accepts invalid output %q", name)
		}
	}
}
