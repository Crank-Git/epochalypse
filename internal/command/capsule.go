package command

import (
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

type capsuleFlags struct {
	state, path, release, id string
	prepared                 string
	objects, bytes           string
	limit                    int
	input                    capsule.Input
}

func runCapsule(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[1] != "create" && args[1] != "publish") {
		_, _ = fmt.Fprintln(stderr, "use capsule create or capsule publish; stop the main server first")
		return 2
	}
	options, err := parseCapsuleFlags(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	manifest, err := executeCapsule(args[1], options)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(manifest); err != nil {
		return 1
	}
	return 0
}

func parseCapsuleFlags(args []string, stderr io.Writer) (capsuleFlags, error) {
	var options capsuleFlags
	flags := flag.NewFlagSet("capsule "+args[1], flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.state, "state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "State directory; stop the main server first")
	flags.StringVar(&options.objects, "max-objects", envDefault("EPOCHALYPSE_MAX_OBJECTS", "100"), "Maximum retained content objects")
	flags.StringVar(&options.bytes, "quota-bytes", envDefault("EPOCHALYPSE_QUOTA_BYTES", "1073741824"), "Maximum retained content bytes and pending reservations")
	flags.IntVar(&options.limit, "plaintext-limit", capsule.DefaultPlaintextLimit, "Maximum plaintext bytes for new capsules")
	mode, kind := "", "text"
	if args[1] == "create" {
		flags.StringVar(&mode, "mode", "", "public, server-held, or tlock; preparation stays on this host")
		flags.StringVar(&kind, "kind", "text", "text (UTF-8) or file (opaque bytes)")
		flags.StringVar(&options.path, "content-file", "", "Local regular file; preparation stays on this host")
		flags.StringVar(&options.prepared, "prepared-output", "", "New canonical tlock draft file for owner-authorized import; excludes plaintext and unwrapped keys")
		flags.StringVar(&options.input.Title, "title", "", "Capsule title")
		flags.StringVar(&options.release, "release-at", "", "Requested release time in RFC 3339 whole seconds")
	} else {
		flags.StringVar(&options.id, "id", "", "Capsule identifier to publish")
	}
	if err := flags.Parse(args[2:]); err != nil {
		return options, err
	}
	if flags.NArg() != 0 || (args[1] == "create" && options.path == "") || (args[1] == "publish" && options.id == "") {
		return options, fmt.Errorf("provide the capsule fields with flags only")
	}
	options.input.Mode, options.input.Kind = capsule.Mode(mode), capsule.Kind(kind)
	if options.prepared != "" && options.input.Mode != capsule.Tlock {
		return options, fmt.Errorf("--prepared-output requires tlock mode")
	}
	if args[1] == "create" {
		release, err := time.Parse(time.RFC3339, options.release)
		if err != nil || release.Nanosecond() != 0 {
			return options, fmt.Errorf("provide --release-at in RFC 3339 whole seconds")
		}
		options.input.ReleaseAt = release
	}
	return options, nil
}

func executeCapsule(action string, options capsuleFlags) (capsule.Manifest, error) {
	limits, err := parseWindowLimits(options.objects, options.bytes)
	if err != nil {
		return capsule.Manifest{}, err
	}
	store, err := capsule.Open(options.state, installation.Limits(limits), options.limit)
	if err != nil {
		return capsule.Manifest{}, err
	}
	defer func() { _ = store.Close() }()
	if action == "publish" {
		record, err := store.Publish(options.id, time.Now())
		return record.Manifest, err
	}
	// The host administrator selects the local content file.
	file, err := os.Open(options.path) // #nosec G304
	if err != nil {
		return capsule.Manifest{}, fmt.Errorf("open capsule content file: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return capsule.Manifest{}, fmt.Errorf("inspect capsule content file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return capsule.Manifest{}, fmt.Errorf("capsule content requires a regular file")
	}
	options.input.Filename = filepath.Base(options.path)
	record, err := store.Create(options.input, file, time.Now())
	if err == nil && options.prepared != "" {
		err = writePreparedCapsule(options.prepared, record)
	}
	return record.Manifest, err
}

func writePreparedCapsule(path string, record capsule.Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode prepared capsule: %w", err)
	}
	// Exclusive creation preserves an existing file and its permissions.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304
	if err != nil {
		return fmt.Errorf("create prepared capsule file: %w", err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write prepared capsule file: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close prepared capsule file: %w", closeErr)
	}
	return nil
}
