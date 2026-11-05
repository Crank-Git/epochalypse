// Package backup authenticates bounded main-service archives before state access.
package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"filippo.io/age"
)

// DefaultMaxBytes permits the default 1 GiB retained installation plus database overhead.
// Operators must select a larger finite limit for larger installations.
const DefaultMaxBytes int64 = 4 << 30

const maxManifestBytes = 64 << 10

// The allowlist excludes agent state and the separate backup identity.
var paths = []string{
	"state/v1/owner.db", "state/v1/windows.db", "config/main.json",
	"keys/window.private", "keys/window.key", "keys/countdown.private", "keys/countdown.key",
	"tls/fullchain.pem", "tls/private.pem",
}

// Limits bounds the complete plaintext stream, including its manifest.
// TempDir selects a trusted parent for private temporary files.
type Limits struct {
	MaxBytes int64
	TempDir  string
}

// File binds one regular file to its original numeric owner and digest.
// SourcePath identifies a private snapshot file and never enters the archive.
type File struct {
	Path       string `json:"path"`
	UID        int    `json:"uid"`
	GID        int    `json:"gid"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	SourcePath string `json:"-"`
}

// Archive preserves the source commit and effective retention limits.
type Archive struct {
	Version    int       `json:"version"`
	Commit     string    `json:"commit"`
	CreatedAt  time.Time `json:"created_at"`
	MaxObjects int       `json:"max_objects"`
	QuotaBytes int64     `json:"quota_bytes"`
	Files      []File    `json:"files"`
	stage      string
}

// Close removes the private files that Open creates.
func (a *Archive) Close() error {
	if a.stage == "" {
		return nil
	}
	return os.RemoveAll(a.stage) // #nosec G703 -- Only MkdirTemp sets the private stage; the manifest excludes it.
}

// Seal streams private snapshot files to a separate operator recipient.
// It rejects invalid metadata, changed files, cancellation, and writer failures.
func Seal(ctx context.Context, dst io.Writer, archive Archive, recipient *age.X25519Recipient, limits Limits) error {
	if recipient == nil {
		return errors.New("backup requires an age recipient")
	}
	manifest, err := archive.manifest(limits)
	if err != nil {
		return err
	}
	stage, err := stageSources(ctx, archive, limits)
	if err != nil {
		return err
	}
	defer func() { _ = stage.Close(); _ = os.Remove(stage.Name()) }()
	writer, err := age.Encrypt(contextWriter{ctx, dst}, recipient)
	if err != nil {
		return fmt.Errorf("start backup encryption: %w", err)
	}
	var prefix [8]byte
	binary.BigEndian.PutUint64(prefix[:], uint64(len(manifest)))
	input := io.MultiReader(bytes.NewReader(prefix[:]), bytes.NewReader(manifest), stage)
	if _, err := io.Copy(writer, contextReader{ctx, input}); err != nil {
		return fmt.Errorf("encrypt backup: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish backup encryption: %w", err)
	}
	return nil
}

func stageSources(ctx context.Context, archive Archive, limits Limits) (_ *os.File, err error) {
	stage, err := os.CreateTemp(limits.TempDir, "epochalypse-backup-*")
	if err != nil {
		return nil, fmt.Errorf("create private backup stage: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stage.Close()
			_ = os.Remove(stage.Name())
		}
	}()
	for _, file := range archive.Files {
		if err = copySource(ctx, stage, file); err != nil {
			return nil, err
		}
	}
	if _, err = stage.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind backup stage: %w", err)
	}
	return stage, nil
}

func copySource(ctx context.Context, dst io.Writer, file File) error {
	if err := regularParent(file.SourcePath); err != nil {
		return err
	}
	info, err := os.Lstat(file.SourcePath)
	if err != nil {
		return fmt.Errorf("inspect backup source: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != file.Size {
		return errors.New("backup source requires a regular file of the declared size")
	}
	src, err := os.Open(file.SourcePath)
	if err != nil {
		return fmt.Errorf("open backup source: %w", err)
	}
	defer func() { _ = src.Close() }()
	opened, err := src.Stat()
	if err != nil {
		return fmt.Errorf("inspect open backup source: %w", err)
	}
	if !os.SameFile(info, opened) {
		return errors.New("backup source changes during open")
	}
	return copyPayload(ctx, dst, src, file)
}

// Open returns private staged files after complete authentication and format checks.
// The caller must call Close, including after installation.
func Open(ctx context.Context, src io.Reader, identity *age.X25519Identity, limits Limits) (result Archive, err error) {
	if identity == nil {
		return result, errors.New("backup requires an age identity")
	}
	if limits.MaxBytes < 1 {
		return result, errors.New("backup requires a positive byte limit")
	}
	stage, err := os.MkdirTemp(limits.TempDir, "epochalypse-restore-*")
	if err != nil {
		return result, fmt.Errorf("create private restore stage: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(stage)
		}
	}()
	plaintext, err := authenticate(ctx, src, identity, limits, stage)
	if err != nil {
		return Archive{}, err
	}
	defer func() { _ = plaintext.Close(); _ = os.Remove(plaintext.Name()) }() // #nosec G703 -- CreateTemp selects the private plaintext path.
	result, err = parseManifest(plaintext, limits)
	if err != nil {
		return Archive{}, err
	}
	if err = extract(ctx, plaintext, &result, stage); err != nil {
		return Archive{}, err
	}
	result.stage = stage
	return result, nil
}

func authenticate(ctx context.Context, src io.Reader, identity *age.X25519Identity, limits Limits, stage string) (_ *os.File, err error) {
	reader, err := age.Decrypt(contextReader{ctx, src}, identity)
	if err != nil {
		return nil, fmt.Errorf("authenticate backup header: %w", err)
	}
	file, err := os.CreateTemp(stage, "plaintext-*")
	if err != nil {
		return nil, fmt.Errorf("create private plaintext stage: %w", err)
	}
	defer func() {
		if err != nil {
			_ = file.Close()
		}
	}()
	// EOF authenticates the final age chunk before the parser sees any plaintext.
	n, err := io.Copy(file, io.LimitReader(contextReader{ctx, reader}, limits.MaxBytes))
	if err != nil {
		return nil, fmt.Errorf("authenticate backup payload: %w", err)
	}
	var extra [1]byte
	_, tailErr := io.ReadFull(contextReader{ctx, reader}, extra[:])
	if tailErr != io.EOF {
		return nil, errors.Join(errors.New("backup authentication or byte limit fails"), tailErr)
	}
	if n == 0 {
		return nil, errors.New("backup plaintext is empty")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind authenticated backup: %w", err)
	}
	return file, nil
}

func parseManifest(src io.Reader, limits Limits) (Archive, error) {
	var prefix [8]byte
	if _, err := io.ReadFull(src, prefix[:]); err != nil {
		return Archive{}, fmt.Errorf("read backup manifest size: %w", err)
	}
	size := binary.BigEndian.Uint64(prefix[:])
	if size == 0 || size > maxManifestBytes {
		return Archive{}, errors.New("backup manifest exceeds the byte limit")
	}
	data := make([]byte, int(size))
	if _, err := io.ReadFull(src, data); err != nil {
		return Archive{}, fmt.Errorf("read backup manifest: %w", err)
	}
	var archive Archive
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&archive); err != nil {
		return Archive{}, errors.New("invalid backup manifest")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Archive{}, errors.New("backup requires one manifest")
	}
	if _, err := archive.manifest(limits); err != nil {
		return Archive{}, err
	}
	return archive, nil
}

func extract(ctx context.Context, src io.Reader, archive *Archive, stage string) error {
	for i := range archive.Files {
		file := &archive.Files[i]
		file.SourcePath = filepath.Join(stage, file.Path)
		if err := os.MkdirAll(filepath.Dir(file.SourcePath), 0700); err != nil {
			return fmt.Errorf("create private file parent: %w", err)
		}
		dst, err := os.OpenFile(file.SourcePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("create private restored file: %w", err)
		}
		copyErr := copyPayload(ctx, dst, io.LimitReader(src, file.Size), *file)
		if err := errors.Join(copyErr, dst.Close()); err != nil {
			return err
		}
	}
	var extra [1]byte
	if _, err := io.ReadFull(src, extra[:]); err != io.EOF {
		return errors.New("backup contains extra payload data")
	}
	return nil
}

func copyPayload(ctx context.Context, dst io.Writer, src io.Reader, file File) error {
	digest := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, digest), io.LimitReader(contextReader{ctx, src}, file.Size))
	if err != nil {
		return fmt.Errorf("copy backup payload: %w", err)
	}
	if n != file.Size || hex.EncodeToString(digest.Sum(nil)) != file.SHA256 {
		return errors.New("backup file size or digest differs")
	}
	return nil
}

func (a Archive) manifest(limits Limits) ([]byte, error) {
	if err := a.validateMetadata(); err != nil {
		return nil, err
	}
	if limits.MaxBytes < 1 {
		return nil, errors.New("backup requires a positive byte limit")
	}
	data, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("encode backup manifest: %w", err)
	}
	if len(data) > maxManifestBytes || int64(len(data))+8 > limits.MaxBytes {
		return nil, errors.New("backup manifest exceeds the byte limit")
	}
	remaining := limits.MaxBytes - int64(len(data)) - 8
	seen := make(map[string]bool, len(paths))
	for _, file := range a.Files {
		if !slices.Contains(paths, file.Path) || seen[file.Path] {
			return nil, errors.New("backup contains an unexpected or duplicate path")
		}
		seen[file.Path] = true
		if file.UID < 0 || file.GID < 0 || file.Size < 1 || file.Size > remaining {
			return nil, errors.New("backup contains invalid file metadata or exceeds the byte limit")
		}
		remaining -= file.Size
		digest, err := hex.DecodeString(file.SHA256)
		if err != nil || len(digest) != sha256.Size {
			return nil, errors.New("backup requires a SHA256 file digest")
		}
	}
	return data, nil
}

func (a Archive) validateMetadata() error {
	if a.Version != 1 || a.CreatedAt.IsZero() || a.MaxObjects < 1 || a.QuotaBytes < 1 {
		return errors.New("unsupported backup metadata")
	}
	if len(a.Commit) != 40 && len(a.Commit) != 64 {
		return errors.New("backup requires a source commit")
	}
	if _, err := hex.DecodeString(a.Commit); err != nil {
		return errors.New("backup requires a hexadecimal source commit")
	}
	if len(a.Files) != len(paths) {
		return errors.New("backup requires every main-service file")
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	src io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.src.Read(data)
}

type contextWriter struct {
	ctx context.Context
	dst io.Writer
}

func (w contextWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.dst.Write(data)
}
