package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
)

func fixture(t *testing.T) Archive {
	t.Helper()
	dir := t.TempDir()
	a := Archive{Version: 1, Commit: strings.Repeat("a", 40), CreatedAt: time.Now().UTC(), MaxObjects: 100, QuotaBytes: 1 << 30}
	for _, path := range paths {
		data := []byte("synthetic private content " + path)
		digest := sha256.Sum256(data)
		source := filepath.Join(dir, filepath.Base(path))
		if err := os.WriteFile(source, data, 0600); err != nil {
			t.Fatal(err)
		}
		a.Files = append(a.Files, File{Path: path, UID: 65534, GID: 65534, SHA256: hex.EncodeToString(digest[:]), Size: int64(len(data)), SourcePath: source})
	}
	return a
}

func TestArchiveRejectsInvalidMetadataBeforeEncryption(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*Archive){
		"version":        func(a *Archive) { a.Version = 2 },
		"time":           func(a *Archive) { a.CreatedAt = time.Time{} },
		"object limit":   func(a *Archive) { a.MaxObjects = 0 },
		"byte quota":     func(a *Archive) { a.QuotaBytes = 0 },
		"commit length":  func(a *Archive) { a.Commit = "short" },
		"commit syntax":  func(a *Archive) { a.Commit = strings.Repeat("z", 40) },
		"missing file":   func(a *Archive) { a.Files = a.Files[1:] },
		"duplicate path": func(a *Archive) { a.Files[1].Path = a.Files[0].Path },
		"traversal":      func(a *Archive) { a.Files[0].Path = "../owner.db" },
		"agent":          func(a *Archive) { a.Files[0].Path = "agent/v1/state.db" },
		"negative owner": func(a *Archive) { a.Files[0].UID = -1 },
		"negative group": func(a *Archive) { a.Files[0].GID = -1 },
		"empty file":     func(a *Archive) { a.Files[0].Size = 0 },
		"digest":         func(a *Archive) { a.Files[0].SHA256 = strings.Repeat("0", 64) },
		"content limit":  func(a *Archive) { a.Files[0].Size = DefaultMaxBytes + 1 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			a := fixture(t)
			mutate(&a)
			var dst bytes.Buffer
			if err := Seal(context.Background(), &dst, a, identity.Recipient(), testLimits(t)); err == nil || dst.Len() != 0 {
				t.Fatal("invalid archive reaches encryption")
			}
		})
	}
}

func TestArchiveRejectsWrongKeysCorruptionAndMalformedDocuments(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	other, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var encrypted bytes.Buffer
	if err := Seal(context.Background(), &encrypted, fixture(t), identity.Recipient(), testLimits(t)); err != nil {
		t.Fatal(err)
	}
	if a, err := Open(context.Background(), bytes.NewReader(encrypted.Bytes()), other, testLimits(t)); err == nil || len(a.Files) != 0 {
		t.Fatal("wrong key exposes state")
	}
	corrupt := bytes.Clone(encrypted.Bytes())
	corrupt[len(corrupt)-1] ^= 1
	if a, err := Open(context.Background(), bytes.NewReader(corrupt), identity, testLimits(t)); err == nil || len(a.Files) != 0 {
		t.Fatal("corruption exposes state")
	}
	valid, err := json.Marshal(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range []string{"bad", string(valid) + " {}", strings.Replace(string(valid), `"version":1`, `"version":2`, 1), strings.Replace(string(valid), `"version":1`, `"unexpected":true,"version":1`, 1)} {
		var dst bytes.Buffer
		w, err := age.Encrypt(&dst, identity.Recipient())
		if err != nil {
			t.Fatal(err)
		}
		var prefix [8]byte
		binary.BigEndian.PutUint64(prefix[:], uint64(len(document)))
		if _, err := w.Write(prefix[:]); err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(document)); err != nil {
			t.Fatal(err)
		}
		if err = w.Close(); err != nil {
			t.Fatal(err)
		}
		if a, err := Open(context.Background(), &dst, identity, testLimits(t)); err == nil || len(a.Files) != 0 {
			t.Fatal("invalid document exposes state")
		}
	}
}

type failedIO struct{}

func (failedIO) Read([]byte) (int, error)  { return 0, io.ErrUnexpectedEOF }
func (failedIO) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestArchiveRejectsCancellationAndIOFailures(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Seal(ctx, io.Discard, fixture(t), identity.Recipient(), testLimits(t)); !errors.Is(err, context.Canceled) {
		t.Fatal("encryption ignores cancellation")
	}
	if _, err := Open(ctx, strings.NewReader(""), identity, testLimits(t)); !errors.Is(err, context.Canceled) {
		t.Fatal("decryption ignores cancellation")
	}
	if err := Seal(context.Background(), failedIO{}, fixture(t), identity.Recipient(), testLimits(t)); err == nil {
		t.Fatal("encryption ignores write failure")
	}
	if _, err := Open(context.Background(), failedIO{}, identity, testLimits(t)); err == nil {
		t.Fatal("decryption ignores read failure")
	}
	if err := Seal(context.Background(), io.Discard, fixture(t), nil, testLimits(t)); err == nil {
		t.Fatal("encryption accepts no recipient")
	}
	if _, err := Open(context.Background(), strings.NewReader(""), nil, testLimits(t)); err == nil {
		t.Fatal("decryption accepts no identity")
	}

}

type countedWriter struct{ calls, fail int }

func (w *countedWriter) Write(data []byte) (int, error) {
	w.calls++
	if w.calls == w.fail {
		return 0, io.ErrClosedPipe
	}
	return len(data), nil
}

func TestArchiveReportsEachEncryptionWriteFailure(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	a := fixture(t)
	setContent(t, &a.Files[0], bytes.Repeat([]byte("synthetic"), 10000))
	w := &countedWriter{}
	if err := Seal(context.Background(), w, a, identity.Recipient(), testLimits(t)); err != nil {
		t.Fatal(err)
	}
	for fail := 1; fail <= w.calls; fail++ {
		if err := Seal(context.Background(), &countedWriter{fail: fail}, a, identity.Recipient(), testLimits(t)); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("write %d loses failure: %v", fail, err)
		}
	}
	a.CreatedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := Seal(context.Background(), io.Discard, a, identity.Recipient(), testLimits(t)); err == nil {
		t.Fatal("invalid timestamp reaches encryption")
	}
}

func TestArchiveAuthenticatesEveryFileBeforeReturn(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	a := fixture(t)
	var encrypted bytes.Buffer
	if err := Seal(context.Background(), &encrypted, a, identity.Recipient(), testLimits(t)); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted.Bytes(), []byte("synthetic private content")) {
		t.Fatal("archive exposes private content")
	}
	restored, err := Open(context.Background(), bytes.NewReader(encrypted.Bytes()), identity, testLimits(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if restored.Commit != a.Commit || len(restored.Files) != len(paths) {
		t.Fatal("archive loses metadata")
	}
	for i, file := range restored.Files {
		if file.Path != a.Files[i].Path || file.SHA256 != a.Files[i].SHA256 || !bytes.Equal(readContent(t, file), readContent(t, a.Files[i])) {
			t.Fatal("archive changes retained data")
		}
	}
	for cut := 0; cut < len(encrypted.Bytes()); cut += 73 {
		result, err := Open(context.Background(), bytes.NewReader(encrypted.Bytes()[:cut]), identity, testLimits(t))
		if err == nil || len(result.Files) != 0 {
			t.Fatal("partial archive exposes content")
		}
	}
}

func testLimits(t *testing.T) Limits {
	t.Helper()
	return Limits{MaxBytes: DefaultMaxBytes, TempDir: t.TempDir()}
}

func setContent(t *testing.T, file *File, data []byte) {
	t.Helper()
	if err := os.WriteFile(file.SourcePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	file.Size = int64(len(data))
	file.SHA256 = hex.EncodeToString(digest[:])
}

func readContent(t *testing.T, file File) []byte {
	t.Helper()
	data, err := os.ReadFile(file.SourcePath)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
