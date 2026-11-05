package command

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/backup"
)

type backupOptions struct {
	source, output, recipient, input, destination, identity, commit string
	maxObjects, uid, gid                                            int
	quota                                                           int64
	limits                                                          backup.Limits
}

func runBackup(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[1] != "create" && args[1] != "restore") {
		_, _ = fmt.Fprintln(stderr, "use backup create or backup restore with explicit paths and commit")
		return 2
	}
	options, err := parseBackupOptions(args, stderr)
	if err != nil {
		return 2
	}
	var archive backup.Archive
	if args[1] == "create" {
		archive, err = createBackup(ctx, options)
	} else {
		archive, err = restoreBackup(ctx, options)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	result := struct {
		Commit     string `json:"commit"`
		MaxObjects int    `json:"max_objects"`
		QuotaBytes int64  `json:"quota_bytes"`
	}{archive.Commit, archive.MaxObjects, archive.QuotaBytes}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		return 1
	}
	return 0
}

func parseBackupOptions(args []string, stderr io.Writer) (backupOptions, error) {
	var options backupOptions
	flags := flag.NewFlagSet("backup "+args[1], flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.commit, "commit", "", "Commit of the separately retained executable")
	flags.Int64Var(&options.limits.MaxBytes, "max-bytes", backup.DefaultMaxBytes, "Finite plaintext archive byte limit")
	flags.StringVar(&options.limits.TempDir, "temp-dir", "", "Trusted temporary directory")
	if args[1] == "create" {
		addBackupCreateFlags(flags, &options)
	} else {
		addBackupRestoreFlags(flags, &options)
	}
	if err := flags.Parse(args[2:]); err != nil {
		return options, err
	}
	valid := options.source != "" && options.output != "" && options.recipient != "" && options.maxObjects > 0 && options.quota > 0
	if args[1] == "restore" {
		valid = options.input != "" && options.destination != "" && options.identity != "" && options.uid >= 0 && options.gid >= 0
	}
	if flags.NArg() != 0 || options.commit == "" || options.limits.MaxBytes < 1 || !valid {
		err := errors.New("backup requires the documented flags and positive limits")
		_, _ = fmt.Fprintln(stderr, err)
		return options, err
	}
	return options, nil
}

func addBackupCreateFlags(flags *flag.FlagSet, options *backupOptions) {
	flags.StringVar(&options.source, "source", "", "Stopped installation with canonical paths")
	flags.StringVar(&options.output, "output", "", "Fresh encrypted archive path")
	flags.StringVar(&options.recipient, "recipient-file", "", "Protected separate age X25519 recipient file")
	flags.IntVar(&options.maxObjects, "max-objects", 0, "Actual source object limit")
	flags.Int64Var(&options.quota, "quota-bytes", 0, "Actual source byte quota")
}

func addBackupRestoreFlags(flags *flag.FlagSet, options *backupOptions) {
	flags.StringVar(&options.input, "input", "", "Protected encrypted archive path")
	flags.StringVar(&options.destination, "destination", "", "Fresh installation path")
	flags.StringVar(&options.identity, "identity-file", "", "Protected separate age X25519 identity file")
	flags.IntVar(&options.uid, "uid", -1, "Destination service numeric user")
	flags.IntVar(&options.gid, "gid", -1, "Destination service numeric group")
}

func createBackup(ctx context.Context, options backupOptions) (result backup.Archive, err error) {
	data, err := readBackupKey(options.recipient)
	if err != nil {
		return result, err
	}
	recipient, err := age.ParseX25519Recipient(data)
	if err != nil {
		return result, errors.New("backup requires a valid age X25519 recipient")
	}
	result, err = backup.Snapshot(ctx, backup.Source{Root: options.source, Commit: options.commit, MaxObjects: options.maxObjects, QuotaBytes: options.quota}, options.limits)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, result.Close()) }()
	err = backup.SealFile(ctx, options.output, result, recipient, options.limits)
	return result, err
}

func restoreBackup(ctx context.Context, options backupOptions) (result backup.Archive, err error) {
	data, err := readBackupKey(options.identity)
	if err != nil {
		return result, err
	}
	identity, err := age.ParseX25519Identity(data)
	if err != nil {
		return result, errors.New("restore requires a valid age X25519 identity")
	}
	file, err := openProtectedBackupFile(options.input)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	result, err = backup.Open(ctx, file, identity, options.limits)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, result.Close()) }()
	err = result.Install(ctx, backup.Destination{Path: options.destination, Commit: options.commit, UID: options.uid, GID: options.gid}, options.limits)
	return result, err
}

func readBackupKey(path string) (string, error) {
	file, err := openProtectedBackupFile(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 1025))
	if err != nil {
		return "", fmt.Errorf("read backup key file: %w", err)
	}
	if len(data) > 1024 {
		return "", errors.New("backup key file exceeds 1024 bytes")
	}
	return strings.TrimSpace(string(data)), nil
}

func openProtectedBackupFile(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect backup file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("backup file requires a regular file with owner-only access")
	}
	file, err := os.Open(path) // #nosec G304 -- The operator selects the protected backup file.
	if err != nil {
		return nil, fmt.Errorf("open backup file: %w", err)
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, errors.Join(errors.New("backup file changes during open"), err)
	}
	return file, nil
}
