package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
)

func TestArchiveSupportsRetainedStateAbove32MiB(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	a := fixture(t)
	setContent(t, &a.Files[0], bytes.Repeat([]byte("s"), 33<<20))
	if err := Seal(context.Background(), io.Discard, a, identity.Recipient(), testLimits(t)); err != nil {
		t.Fatalf("supported retained state fails: %v", err)
	}
}

func TestArchiveStreamsLargePayloadWithBoundedReads(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	a := fixture(t)
	file, err := os.OpenFile(a.Files[0].SourcePath, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	const size = 33 << 20
	if _, err := io.CopyN(io.MultiWriter(file, digest), zeroReader{}, size); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	a.Files[0].Size = size
	a.Files[0].SHA256 = hex.EncodeToString(digest.Sum(nil))
	limits := testLimits(t)
	encrypted, err := os.CreateTemp(t.TempDir(), "encrypted-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = encrypted.Close() }()
	if err := Seal(context.Background(), encrypted, a, identity.Recipient(), limits); err != nil {
		t.Fatal(err)
	}
	if _, err := encrypted.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	result, err := Open(context.Background(), smallReads{encrypted}, identity, limits)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.Close() }()
	if result.Files[0].Size != size {
		t.Fatal("large payload changes size")
	}
	for _, file := range result.Files {
		info, err := os.Stat(file.SourcePath)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("restored file permits other users")
		}
	}
	info, err := os.Stat(result.stage)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("restore stage permits other users")
	}
	if err := result.Close(); err != nil {
		t.Fatal(err)
	}
	assertEmpty(t, limits.TempDir)
}

type zeroReader struct{}

func (zeroReader) Read(data []byte) (int, error) { clear(data); return len(data), nil }

type smallReads struct{ io.Reader }

func (r smallReads) Read(data []byte) (int, error) {
	if len(data) > 128<<10 {
		panic("archive requests an unbounded read")
	}
	return r.Reader.Read(data)
}

func TestArchiveRejectsBoundsAndRemovesTemporaryPlaintext(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	a := fixture(t)
	limits := testLimits(t)
	var encrypted bytes.Buffer
	if err := Seal(context.Background(), &encrypted, a, identity.Recipient(), limits); err != nil {
		t.Fatal(err)
	}
	assertEmpty(t, limits.TempDir)
	for _, bound := range []int64{-1, 0, 1, 100, 1000} {
		limits.MaxBytes = bound
		if _, err := Open(context.Background(), bytes.NewReader(encrypted.Bytes()), identity, limits); err == nil {
			t.Fatal("restore ignores byte limit")
		}
		assertEmpty(t, limits.TempDir)
		if err := Seal(context.Background(), io.Discard, a, identity.Recipient(), limits); err == nil {
			t.Fatal("backup ignores byte limit")
		}
		assertEmpty(t, limits.TempDir)
	}
	limits.MaxBytes = DefaultMaxBytes
	setContent(t, &a.Files[0], []byte("changed private content"))
	a.Files[0].SHA256 = strings.Repeat("0", 64)
	if err := Seal(context.Background(), io.Discard, a, identity.Recipient(), limits); err == nil {
		t.Fatal("backup accepts a changed source")
	}
	assertEmpty(t, limits.TempDir)
	if err := os.Remove(a.Files[0].SourcePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a.Files[1].SourcePath, a.Files[0].SourcePath); err != nil {
		t.Fatal(err)
	}
	if err := Seal(context.Background(), io.Discard, a, identity.Recipient(), limits); err == nil {
		t.Fatal("backup accepts a source link")
	}
	assertEmpty(t, limits.TempDir)
}

func assertEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("temporary plaintext remains")
	}
}

func TestArchiveAuthenticatesBeforeItParsesTheManifest(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var encrypted bytes.Buffer
	w, err := age.Encrypt(&encrypted, identity.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	// An oversized manifest must remain unseen when the final authentication fails.
	var prefix [8]byte
	binary.BigEndian.PutUint64(prefix[:], maxManifestBytes+1)
	if _, err := w.Write(prefix[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(w, zeroReader{}, 128<<10); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	corrupt := bytes.Clone(encrypted.Bytes())
	corrupt[len(corrupt)-1] ^= 1
	limits := testLimits(t)
	if _, err := Open(context.Background(), bytes.NewReader(corrupt), identity, limits); err == nil || !strings.Contains(err.Error(), "authenticate backup payload") {
		t.Fatalf("parser sees unauthenticated data: %v", err)
	}
	assertEmpty(t, limits.TempDir)
	if _, err := Open(context.Background(), bytes.NewReader(encrypted.Bytes()), identity, limits); err == nil || !strings.Contains(err.Error(), "manifest exceeds") {
		t.Fatalf("authenticated manifest bypasses bounds: %v", err)
	}
	assertEmpty(t, limits.TempDir)
	limits.TempDir = filepath.Join(t.TempDir(), "absent", "parent")
	if _, err := Open(context.Background(), strings.NewReader(""), identity, limits); err == nil {
		t.Fatal("restore ignores stage creation failure")
	}
	if err := Seal(context.Background(), io.Discard, fixture(t), identity.Recipient(), limits); err == nil {
		t.Fatal("backup ignores stage creation failure")
	}
}

func TestArchiveRejectsMalformedFramingAndPayloads(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	a := fixture(t)
	limits := testLimits(t)
	manifest, err := a.manifest(limits)
	if err != nil {
		t.Fatal(err)
	}
	var prefix [8]byte
	binary.BigEndian.PutUint64(prefix[:], uint64(len(manifest)))
	valid := append(bytes.Clone(prefix[:]), manifest...)
	for _, file := range a.Files {
		valid = append(valid, readContent(t, file)...)
	}
	changed := bytes.Clone(valid)
	changed[len(changed)-1] ^= 1
	for _, plaintext := range [][]byte{nil, {0}, make([]byte, 8), prefix[:], valid[:len(valid)-1], append(bytes.Clone(valid), 1), changed} {
		var encrypted bytes.Buffer
		w, err := age.Encrypt(&encrypted, identity.Recipient())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(plaintext); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if result, err := Open(context.Background(), &encrypted, identity, limits); err == nil || len(result.Files) != 0 {
			t.Fatal("malformed archive exposes files")
		}
		assertEmpty(t, limits.TempDir)
	}
	for _, digest := range []string{"x", "00", strings.Repeat("a", maxManifestBytes)} {
		a.Files[0].SHA256 = digest
		if _, err := a.manifest(limits); err == nil {
			t.Fatal("invalid digest reaches encryption")
		}
	}
	if err := (&Archive{}).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveReportsPrivateStageFailures(t *testing.T) {
	ctx := context.Background()
	a := fixture(t)
	if err := os.Remove(a.Files[0].SourcePath); err != nil {
		t.Fatal(err)
	}
	if err := copySource(ctx, io.Discard, a.Files[0]); err == nil {
		t.Fatal("missing source passes")
	}
	for _, conflict := range []string{"state", "state/v1/owner.db"} {
		stage := t.TempDir()
		path := filepath.Join(stage, conflict)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := extract(ctx, strings.NewReader(""), &a, stage); err == nil {
			t.Fatal("extraction ignores a private file conflict")
		}
	}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var encrypted bytes.Buffer
	if err := Seal(ctx, &encrypted, fixture(t), identity.Recipient(), testLimits(t)); err != nil {
		t.Fatal(err)
	}
	stage := t.TempDir()
	if err := os.Remove(stage); err != nil {
		t.Fatal(err)
	}
	if _, err := authenticate(ctx, &encrypted, identity, testLimits(t), stage); err == nil {
		t.Fatal("missing private stage passes")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := (contextWriter{canceled, io.Discard}).Write(nil); err == nil {
		t.Fatal("writer ignores cancellation")
	}
}
