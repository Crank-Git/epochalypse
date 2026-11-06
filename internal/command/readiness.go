package command

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/Crank-Git/epochalypse/internal/deployment"
	"github.com/Crank-Git/epochalypse/internal/switchclient"
)

func runReadiness(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("readiness", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("inventory", "", "Local deployment inventory file")
	production := flags.Bool("production", false, "Require conditional production readiness")
	recovery := flags.String("recovery-bundle", "", "Verify released public recovery without printing content")
	if flags.Parse(args) != nil {
		return 2
	}
	if flags.NArg() != 0 || *path == "" {
		_, _ = fmt.Fprintln(stderr, "provide --inventory")
		return 2
	}
	data, err := readBoundedFile(*path, 64<<10)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "read deployment inventory")
		return 1
	}
	inventory, err := deployment.ReadInventory(bytes.NewReader(data))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	report, err := deployment.Check(context.Background(), inventory, nil)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if *recovery != "" {
		report.PublicRecoveryVerified = verifyReadinessRecovery(inventory, *recovery)
	}
	if json.NewEncoder(stdout).Encode(report) != nil {
		return 1
	}
	if !report.Ready || (*production && !report.ProductionReady) || (*recovery != "" && !report.PublicRecoveryVerified) {
		return 1
	}
	return 0
}

func verifyReadinessRecovery(inventory deployment.Inventory, path string) bool {
	data, err := readBoundedFile(path, 16<<20)
	if err != nil {
		return false
	}
	bundle, err := switchclient.ReadBundle(bytes.NewReader(data), inventory.OwnerKey)
	if err != nil || !bytes.Equal(bundle.Plan.Certificate, inventory.Certificate) || !bytes.Equal(bundle.Plan.CertificateSignature, inventory.CertificateSignature) {
		return false
	}
	pins := map[string][32]byte{}
	for _, s := range inventory.Agents {
		pin, _ := hex.DecodeString(s.TLSPin)
		pins[s.Endpoint] = [32]byte(pin)
	}
	plaintext, err := switchclient.Recover(context.Background(), bundle, inventory.OwnerKey, &switchclient.HTTPSReplica{Pins: pins})
	clear(plaintext)
	return err == nil
}
