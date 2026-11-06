package command

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchclient"
)

func runSwitchCheckin(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("switch checkin", flag.ContinueOnError)
	flags.SetOutput(stderr)
	options := armOptions{}
	flags.StringVar(&options.planPath, "plan", "", "Owner-only JSON plan file")
	flags.StringVar(&options.keyPath, "owner-key", "", "Owner-only base64 Ed25519 private key file")
	flags.StringVar(&options.pinsPath, "pins", "", "JSON map of HTTPS endpoints to TLS SPKI SHA256 pins")
	journal := flags.String("journal", "", "Owner-local client journal directory")
	evidence := flags.String("evidence", "", "Public renewal evidence output file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || options.planPath == "" || options.keyPath == "" || options.pinsPath == "" || *journal == "" {
		_, _ = fmt.Fprintln(stderr, "provide --plan, --owner-key, --journal, and --pins")
		return 2
	}
	inputs, err := loadArmInputs(options)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := switchclient.Renew(ctx, *journal, inputs.plan, inputs.key, &switchclient.HTTPSReplica{Pins: inputs.pins}, time.Now())
	if *evidence != "" && len(result.Request.Body) > 0 {
		if exportErr := writeRenewalEvidence(*evidence, result); exportErr != nil {
			_, _ = fmt.Fprintln(stderr, exportErr)
			return writeRenewalResult(stdout, stderr, result, exportErr)
		}
	}
	return writeRenewalResult(stdout, stderr, result, err)
}

func writeRenewalEvidence(path string, result switchclient.Renewal) error {
	data, err := json.Marshal(result.Evidence)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600) // #nosec G306,G304 -- The owner selects a public proof export path.
}

func writeRenewalResult(stdout, stderr io.Writer, result switchclient.Renewal, err error) int {
	if err != nil {
		if !result.Confirmed && len(result.Request.Body) > 0 {
			if encodeErr := json.NewEncoder(stdout).Encode(result); encodeErr != nil {
				return 1
			}
		}
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		return 1
	}
	if !result.Confirmed {
		_, _ = fmt.Fprintln(stderr, "renewal remains uncertain; retry the retained signed intent")
		return 1
	}
	return 0
}

func runSwitchJournalStatus(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("switch status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	plan := flags.String("plan", "", "Owner-only JSON plan file")
	keyPath := flags.String("owner-key", "", "Owner-only base64 Ed25519 private key file")
	journal := flags.String("journal", "", "Owner-local client journal directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *plan == "" || *keyPath == "" || *journal == "" {
		_, _ = fmt.Fprintln(stderr, "provide --plan, --owner-key, and --journal")
		return 2
	}
	p, err := readPrivatePlan(*plan)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	key, err := readOwnerPrivateKey(*keyPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	result, err := switchclient.RenewalStatus(*journal, p, key.Public().(ed25519.PublicKey))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		return 1
	}
	return 0
}
