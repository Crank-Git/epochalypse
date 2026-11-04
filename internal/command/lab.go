package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/trust"
)

func runLab(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[1] != "verify" {
		_, _ = fmt.Fprintln(stderr, "use lab verify --zone --address [--anchor-file]")
		return 2
	}
	flags := flag.NewFlagSet("lab verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	zone := flags.String("zone", "", "Canonical experimental zone")
	address := flags.String("address", "127.0.0.1:1053", "Authority IP address and port")
	path := flags.String("anchor-file", "", "One public DNSKEY or SHA-256 DS record")
	if err := flags.Parse(args[2:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *zone == "" {
		_, _ = fmt.Fprintln(stderr, "set --zone and use flags only")
		return 2
	}
	var anchor dns.RR
	if *path != "" {
		var err error
		anchor, err = readAnchor(*path, *zone)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
	}
	result := trust.Verify(context.Background(), *address, *zone, anchor, time.Now())
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		return 1
	}
	if result.Status != "verified" {
		return 1
	}
	return 0
}

func readAnchor(path, zone string) (dns.RR, error) {
	// The host administrator selects a public anchor file.
	file, err := os.Open(path) // #nosec G304
	if err != nil {
		return nil, fmt.Errorf("open public anchor: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return nil, fmt.Errorf("require a regular public anchor file of at most 4096 bytes")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return nil, fmt.Errorf("read public anchor: %w", err)
	}
	return trust.ParseAnchor(string(data), zone)
}
