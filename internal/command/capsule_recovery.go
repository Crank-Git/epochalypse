package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/installation"
)

type recoveryFlags struct {
	state, id, confirm, bundle, output, signature string
	objects, bytes                                string
	limit                                         int
}

func runCapsuleRecovery(args []string, stdout, stderr io.Writer) int {
	options, err := parseRecoveryFlags(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	material, err := executeRecovery(args[1], options)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(struct {
		Result string `json:"result"`
	}{material}); err != nil {
		return 1
	}
	return 0
}

func parseRecoveryFlags(args []string, stderr io.Writer) (recoveryFlags, error) {
	var options recoveryFlags
	flags := flag.NewFlagSet("capsule "+args[1], flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.IntVar(&options.limit, "plaintext-limit", capsule.DefaultPlaintextLimit, "Maximum plaintext bytes")
	if args[1] == "open" {
		flags.StringVar(&options.bundle, "bundle", "", "Canonical recovery bundle file")
		flags.StringVar(&options.signature, "beacon-signature-file", "", "Optional raw 48-byte beacon signature for offline verification")
	} else {
		flags.StringVar(&options.state, "state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "State directory; stop the main server first")
		flags.StringVar(&options.objects, "max-objects", envDefault("EPOCHALYPSE_MAX_OBJECTS", "100"), "Maximum retained objects")
		flags.StringVar(&options.bytes, "quota-bytes", envDefault("EPOCHALYPSE_QUOTA_BYTES", "1073741824"), "Maximum retained bytes")
		flags.StringVar(&options.id, "id", "", "Capsule identifier")
	}
	if args[1] == "delete" {
		flags.StringVar(&options.confirm, "confirm", "", "Repeat the exact identifier; public copies and backups remain outside your control")
	} else {
		flags.StringVar(&options.output, "output", "", "New owner-only output file; existing files remain unchanged")
	}
	if err := flags.Parse(args[2:]); err != nil {
		return options, err
	}
	if flags.NArg() != 0 || !validRecoveryOptions(args[1], options) {
		return options, fmt.Errorf("provide the required capsule fields with flags only")
	}
	return options, nil
}

func validRecoveryOptions(action string, options recoveryFlags) bool {
	if options.limit <= 0 {
		return false
	}
	if action == "open" {
		return options.bundle != "" && options.output != ""
	}
	if action == "delete" {
		return options.id != "" && options.confirm == options.id
	}
	return options.id != "" && options.output != ""
}

func executeRecovery(action string, options recoveryFlags) (string, error) {
	if action == "open" {
		return openRecovery(options)
	}
	limits, err := parseWindowLimits(options.objects, options.bytes)
	if err != nil {
		return "", err
	}
	store, err := capsule.Open(options.state, installation.Limits(limits), options.limit)
	if err != nil {
		return "", err
	}
	defer func() { _ = store.Close() }()
	if action == "delete" {
		err := store.Delete(context.Background(), options.id, options.confirm)
		return "Local content deleted. Public copies and backups remain outside your control.", err
	}
	bundle, err := store.Bundle(options.id, time.Now().UTC().Truncate(time.Second))
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	if err := writeRecoveryFile(options.output, data); err != nil {
		return "", err
	}
	if bundle.Record.Mode == capsule.Tlock {
		return "beacon-wrapper: recovery needs the pinned verified beacon; availability can be delayed", nil
	}
	if bundle.ReleasedKey == "" {
		return "ciphertext-only: recovery still needs the server-held key; a lost key has no automatic recovery", nil
	}
	return "released-key: this recovery bundle contains the released content key", nil
}

func openRecovery(options recoveryFlags) (string, error) {
	file, err := os.Open(options.bundle) // #nosec G304 -- The host administrator selects the bundle file.
	if err != nil {
		return "", fmt.Errorf("open recovery bundle: %w", err)
	}
	defer func() { _ = file.Close() }()
	bundle, err := capsule.DecodeBundle(file, options.limit)
	if err != nil {
		return "", err
	}
	source, err := recoveryBeacon(options.signature)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	plaintext, err := bundle.Open(ctx, source, time.Now().UTC().Truncate(time.Second))
	defer clear(plaintext)
	if err != nil {
		return "", err
	}
	if err := writeRecoveryFile(options.output, plaintext); err != nil {
		return "", err
	}
	return "Authenticated content saved.", nil
}

type retainedBeacon []byte

func (signature retainedBeacon) Fetch(context.Context, uint64) ([]byte, error) { return signature, nil }

func recoveryBeacon(path string) (capsule.BeaconSource, error) {
	if path == "" {
		return capsule.QuicknetRelay(), nil
	}
	file, err := os.Open(path) // #nosec G304 -- The host administrator selects the signature file.
	if err != nil {
		return nil, fmt.Errorf("open beacon signature: %w", err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 49))
	if err != nil || len(data) != 48 {
		return nil, capsule.ErrIntegrity
	}
	return retainedBeacon(data), nil
}

func writeRecoveryFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".epochalypse-recovery-*")
	if err != nil {
		return fmt.Errorf("create recovery output: %w", err)
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write recovery output: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close recovery output: %w", closeErr)
	}
	// A same-directory hard link exposes only complete bytes and never replaces an existing path.
	if err := os.Link(file.Name(), path); err != nil {
		return fmt.Errorf("publish recovery output: %w", err)
	}
	return nil
}
