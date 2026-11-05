package backup

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/service"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/window"
	bolt "go.etcd.io/bbolt"
)

// Destination selects a fresh installation and its service identity.
// Commit must identify the separately retained executable for this archive.
type Destination struct {
	Path   string
	Commit string
	UID    int
	GID    int
}

// Install validates authenticated state before it publishes a fresh installation.
func (a Archive) Install(ctx context.Context, destination Destination, limits Limits) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if destination.Commit != a.Commit || destination.Commit == "" {
		return errors.New("restore requires the matching executable commit")
	}
	if destination.Path == "" || !validIdentity(destination.UID, destination.GID) {
		return errors.New("restore requires a fresh destination and numeric service identity")
	}
	if _, err := a.manifest(limits); err != nil {
		return err
	}
	if err := a.ValidateOwner(ctx); err != nil {
		return err
	}
	if err := a.validateFiles(ctx); err != nil {
		return err
	}
	settings, signer, err := a.validateConfiguration()
	if err != nil {
		return err
	}
	if err := a.validateInstallation(ctx, settings, signer); err != nil {
		return err
	}
	return a.publish(ctx, destination.Path, destination.UID, destination.GID)
}

func (a Archive) validateFiles(ctx context.Context) error {
	root, err := os.OpenRoot(a.stage)
	if err != nil {
		return fmt.Errorf("open authenticated installation: %w", err)
	}
	defer func() { _ = root.Close() }()
	for _, file := range a.Files {
		if !validIdentity(file.UID, file.GID) {
			return errors.New("restore requires supported numeric file ownership")
		}
		if file.SourcePath != filepath.Join(a.stage, file.Path) {
			return errors.New("restore requires canonical authenticated paths")
		}
		info, err := inspectSource(root, file.Path)
		if err != nil {
			return err
		}
		if info.Size() != file.Size {
			return errors.New("authenticated installation size differs")
		}
		if err := copySource(ctx, io.Discard, file); err != nil {
			return err
		}
	}
	return nil
}

func (a Archive) validateConfiguration() (service.Settings, *window.Signer, error) {
	settings, err := service.ReadSettings(filepath.Join(a.stage, "config/main.json"))
	if err != nil {
		return settings, nil, errors.New("restore requires supported main configuration")
	}
	config := service.Config{Mode: "server", Environment: "lab", HTTPAddr: "127.0.0.1:8080", DNSAddr: "127.0.0.1:1053", StateDir: filepath.Join(a.stage, "state"), Zone: settings.Zone, CountdownZone: settings.CountdownZone, NameServer: settings.NameServer, PublicURL: settings.PublicURL, Resolvers: settings.Resolvers, CapsulePlaintextLimit: settings.CapsulePlaintextLimit, TLSCert: filepath.Join(a.stage, "tls/fullchain.pem"), TLSKey: filepath.Join(a.stage, "tls/private.pem"), KeyFile: filepath.Join(a.stage, "keys/window.private"), CountdownKeyFile: filepath.Join(a.stage, "keys/countdown.private")}
	if config.Validate() != nil {
		return settings, nil, errors.New("restore requires valid main configuration")
	}
	signer, err := window.LoadSigner(settings.Zone, config.KeyFile)
	if err != nil {
		return settings, nil, errors.New("restore requires the original window key pair")
	}
	other, err := countdown.LoadSigner(settings.CountdownZone, config.CountdownKeyFile)
	if err != nil {
		return settings, nil, errors.New("restore requires the separate countdown key pair")
	}
	pair, err := tls.LoadX509KeyPair(config.TLSCert, config.TLSKey)
	if err != nil {
		return settings, nil, errors.New("restore requires a matching TLS certificate and key")
	}
	windowKey, countdownKey := signer.PublicKey().PublicKey, other.PublicKey().PublicKey
	if windowKey == countdownKey {
		return settings, nil, errors.New("restore requires separate DNSSEC keys")
	}
	if public, ok := pair.Leaf.PublicKey.(*ecdsa.PublicKey); ok {
		point, err := public.Bytes()
		if err != nil {
			return settings, nil, errors.New("restore requires a supported TLS public key")
		}
		encoded := base64.StdEncoding.EncodeToString(point[1:])
		if encoded == windowKey || encoded == countdownKey {
			return settings, nil, errors.New("restore requires a separate TLS key")
		}
	}
	return settings, signer, nil
}

func (a Archive) validateInstallation(ctx context.Context, settings service.Settings, signer *window.Signer) (err error) {
	file := File{}
	for _, item := range a.Files {
		if item.Path == "state/v1/windows.db" {
			file = item
		}
	}
	root, err := os.OpenRoot(a.stage)
	if err != nil {
		return fmt.Errorf("open authenticated installation stage: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	info, err := inspectSource(root, file.Path)
	if err != nil {
		return err
	}
	db, err := bolt.Open(file.Path, 0600, &bolt.Options{ReadOnly: true, Timeout: 100 * time.Millisecond, OpenFile: func(name string, flag int, mode os.FileMode) (*os.File, error) {
		src, err := openExisting(root, name, flag, mode, info)
		if err != nil {
			return nil, err
		}
		if err := copyPayload(ctx, io.Discard, src, file); err != nil {
			_ = src.Close()
			return nil, err
		}
		return src, nil
	}})
	if err != nil {
		return fmt.Errorf("open authenticated installation database: %w", err)
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	return db.View(func(tx *bolt.Tx) error { return a.validateDomains(ctx, tx, settings, signer) })
}

func (a Archive) validateDomains(ctx context.Context, tx *bolt.Tx, settings service.Settings, signer *window.Signer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := installation.ValidateSnapshotLayout(tx); err != nil {
		return err
	}
	limits := installation.Limits{MaxObjects: a.MaxObjects, MaxBytes: a.QuotaBytes}
	if err := (&installation.Store{Limits: limits}).Admit(tx, 0, 0); err != nil {
		return err
	}
	if err := window.ValidateSnapshot(tx, signer, limits); err != nil {
		return err
	}
	plaintextLimit := settings.CapsulePlaintextLimit
	if plaintextLimit == 0 {
		plaintextLimit = capsule.DefaultPlaintextLimit
	}
	if err := capsule.ValidateSnapshot(tx, plaintextLimit); err != nil {
		return err
	}
	if err := countdown.ValidateSnapshot(tx); err != nil {
		return err
	}
	if err := switches.ValidateSnapshot(tx); err != nil {
		return err
	}
	return ctx.Err()
}
