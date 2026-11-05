package switchclient

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

const maxBundleBytes = 16 << 20

// Bundle retains public enrollment, encrypted content, and prepared receipts.
type Bundle struct {
	Version  int
	Plan     PublicPlan
	Prepared []switchwire.Envelope
}

// PublicPlan omits the private prepare request and encrypted shares.
type PublicPlan struct {
	Certificate, CertificateSignature, Ciphertext []byte
	State                                         switchwire.State
}

func publicPlan(p Plan, state switchwire.State) PublicPlan {
	return PublicPlan{Certificate: p.Certificate, CertificateSignature: p.CertificateSignature, Ciphertext: p.Ciphertext, State: state}
}

// ReadBundle validates one bounded bundle against the owner's pinned public key.
func ReadBundle(r io.Reader, owner ed25519.PublicKey) (Bundle, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBundleBytes+1))
	if err != nil {
		return Bundle{}, fmt.Errorf("read recovery bundle: %w", err)
	}
	if len(data) > maxBundleBytes {
		return Bundle{}, errors.New("recovery bundle exceeds limit")
	}
	var b Bundle
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&b) != nil || d.Decode(&struct{}{}) != io.EOF {
		return Bundle{}, errors.New("invalid recovery bundle encoding")
	}
	if err := validateBundle(b, owner); err != nil {
		return Bundle{}, err
	}
	return b, nil
}

func validateBundle(b Bundle, owner ed25519.PublicKey) error {
	if b.Version != 1 {
		return errors.New("unsupported recovery bundle version")
	}
	e, err := switchwire.AdmitCertificate(b.Plan.Certificate, b.Plan.CertificateSignature, owner)
	if err != nil {
		return err
	}
	if err := switchcontent.ValidateBinding(b.Plan.Ciphertext, b.Plan.State.Key, b.Plan.State.Content); err != nil {
		return err
	}
	if !switchwire.Confirm(b.Plan.State, b.Prepared, e, switchwire.PhasePrepared) {
		return errors.New("recovery bundle lacks three prepared receipts")
	}
	return nil
}

// ExportBundle persists and reopens the bundle before arm can proceed.
// An existing file must contain the exact same bundle for an idempotent retry.
func ExportBundle(path string, b Bundle, owner ed25519.PublicKey) error {
	if err := validateBundle(b, owner); err != nil {
		return err
	}
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	if len(data) > maxBundleBytes {
		return errors.New("recovery bundle exceeds limit")
	}
	if err := persistExact(path, data); err != nil {
		return err
	}
	return verifyExport(path, data, owner)
}

func persistExact(path string, data []byte) error {
	if len(data) > maxBundleBytes {
		return errors.New("owner intent exceeds limit")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- The owner chooses the local export path.
	if errors.Is(err, os.ErrExist) {
		return verifyExactFile(path, data)
	}
	if err != nil {
		return fmt.Errorf("create recovery bundle: %w", err)
	}
	_, writeErr := f.Write(data)
	err = errors.Join(writeErr, f.Sync(), f.Close())
	if err != nil {
		return fmt.Errorf("persist recovery bundle: %w", err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	if err = errors.Join(directory.Sync(), directory.Close()); err != nil {
		return err
	}
	return verifyExactFile(path, data)
}

func verifyExactFile(path string, expected []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("owner intent must be an owner-only regular file")
	}
	f, err := os.Open(path) // #nosec G304 -- The owner chooses the local export path.
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err = f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("recovery bundle must be an owner-only regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBundleBytes+1))
	if err != nil {
		return err
	}
	if !bytes.Equal(data, expected) {
		return errors.New("existing recovery bundle differs from prepared content")
	}
	return nil
}

func verifyExport(path string, expected []byte, owner ed25519.PublicKey) error {
	if err := verifyExactFile(path, expected); err != nil {
		return err
	}
	_, err := ReadBundle(bytes.NewReader(expected), owner)
	return err
}

func resumePrepared(path string, p Plan, owner ed25519.PublicKey) ([]switchwire.Envelope, error) {
	f, err := os.Open(path) // #nosec G304 -- The owner chooses the local export path.
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := ReadBundle(f, owner)
	if err != nil {
		return nil, err
	}
	frame, err := switchwire.DecodeFrame(p.Prepare.Body)
	if err != nil {
		return nil, err
	}
	expected, err := json.Marshal(publicPlan(p, frame.State))
	if err != nil {
		return nil, err
	}
	stored, err := json.Marshal(b.Plan)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(expected, stored) {
		return nil, errors.New("recovery bundle differs from owner intent")
	}
	return b.Prepared, nil
}
