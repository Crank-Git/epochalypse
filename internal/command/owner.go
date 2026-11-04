package command

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Crank-Git/epochalypse/internal/owner"
)

func runOwner(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[1] != "init" && args[1] != "recover") {
		_, _ = fmt.Fprintln(stderr, "use owner init or owner recover with --password-file")
		return 2
	}
	flags := flag.NewFlagSet("owner "+args[1], flag.ContinueOnError)
	flags.SetOutput(stderr)
	state := flags.String("state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "State directory")
	passwordFile := flags.String("password-file", "", "Owner-only file with the new password")
	if err := flags.Parse(args[2:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *passwordFile == "" {
		_, _ = fmt.Fprintln(stderr, "set --password-file and use flags only")
		return 2
	}
	if err := changeOwner(*state, *passwordFile, args[1]); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	message := "Owner created"
	if args[1] == "recover" {
		message = "Owner recovered; sessions revoked"
	}
	if _, err := fmt.Fprintln(stdout, message); err != nil {
		return 1
	}
	return 0
}

func changeOwner(state, path, action string) error {
	// The host administrator selects the credential file.
	file, err := os.Open(path) // #nosec G304
	if err != nil {
		return fmt.Errorf("open password file: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect password file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("password file requires owner-only access")
	}
	password, err := io.ReadAll(io.LimitReader(file, 1026))
	if err != nil {
		return fmt.Errorf("read password file: %w", err)
	}
	password = bytes.TrimSuffix(password, []byte("\n"))
	store, err := owner.Open(state)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	if action == "recover" {
		return store.Recover(password)
	}
	return store.Initialize(password)
}
