// Package backup authenticates bounded main-service archives before state access.
package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"filippo.io/age"
)

// MaxBytes bounds the complete plaintext archive before JSON allocation.
const MaxBytes = 64 << 20

// The allowlist excludes agent state and the separate backup identity.
var paths = []string{
	"state/v1/owner.db", "state/v1/windows.db", "config/main.json",
	"keys/window.private", "keys/window.key", "keys/countdown.private", "keys/countdown.key",
	"tls/fullchain.pem", "tls/private.pem",
}

// File binds one regular file to its original numeric owner and digest.
type File struct {
	Path   string `json:"path"`
	UID    int    `json:"uid"`
	GID    int    `json:"gid"`
	SHA256 string `json:"sha256"`
	Data   []byte `json:"data"`
}

// Archive preserves the source commit and effective retention limits.
type Archive struct {
	Version    int       `json:"version"`
	Commit     string    `json:"commit"`
	CreatedAt  time.Time `json:"created_at"`
	MaxObjects int       `json:"max_objects"`
	QuotaBytes int64     `json:"quota_bytes"`
	Files      []File    `json:"files"`
}

// Seal encrypts a validated archive to a separate operator recipient.
func Seal(ctx context.Context, dst io.Writer, archive Archive, recipient *age.X25519Recipient) error {
	if recipient == nil {
		return errors.New("backup requires an age recipient")
	}
	if err := archive.validate(); err != nil {
		return err
	}
	// JSON expands byte slices to base64. Bound source data before that allocation.
	data, err := json.Marshal(archive)
	if err != nil {
		return fmt.Errorf("encode backup: %w", err)
	}
	writer, err := age.Encrypt(contextWriter{ctx, dst}, recipient)
	if err != nil {
		return fmt.Errorf("start backup encryption: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("encrypt backup: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish backup encryption: %w", err)
	}
	return nil
}

// Open returns an archive only after the complete age stream authenticates.
func Open(ctx context.Context, src io.Reader, identity *age.X25519Identity) (Archive, error) {
	if identity == nil {
		return Archive{}, errors.New("backup requires an age identity")
	}
	encrypted, err := boundedRead(ctx, src, MaxBytes+(1<<20))
	if err != nil {
		return Archive{}, err
	}
	reader, err := age.Decrypt(bytes.NewReader(encrypted), identity)
	if err != nil {
		return Archive{}, errors.New("backup authentication fails")
	}
	data, err := boundedRead(ctx, reader, MaxBytes)
	if err != nil {
		return Archive{}, errors.New("backup authentication or byte limit fails")
	}
	// No file data escapes until the final age chunk and the complete format pass.
	var archive Archive
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&archive); err != nil {
		return Archive{}, errors.New("invalid backup document")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Archive{}, errors.New("backup requires one document")
	}
	if err := archive.validate(); err != nil {
		return Archive{}, err
	}
	return archive, nil
}

func (a Archive) validate() error {
	if err := a.validateMetadata(); err != nil {
		return err
	}
	seen := make(map[string]bool, len(paths))
	total := 0
	for _, file := range a.Files {
		if !slices.Contains(paths, file.Path) || seen[file.Path] {
			return errors.New("backup contains an unexpected or duplicate path")
		}
		seen[file.Path] = true
		if file.UID < 0 || file.GID < 0 || len(file.Data) == 0 {
			return errors.New("backup contains invalid file metadata")
		}
		if len(file.Data) > MaxBytes/2-total {
			return errors.New("backup exceeds the content byte limit")
		}
		total += len(file.Data)
		digest := sha256.Sum256(file.Data)
		if file.SHA256 != hex.EncodeToString(digest[:]) {
			return errors.New("backup file digest differs")
		}
	}
	return nil
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

func boundedRead(ctx context.Context, src io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, src}, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read backup: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, errors.New("backup exceeds the byte limit")
	}
	return data, nil
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
