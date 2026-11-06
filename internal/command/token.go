package command

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Crank-Git/epochalypse/internal/owner"
)

func runToken(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[1] != "create" {
		_, _ = fmt.Fprintln(stderr, "use token create with --name, --scopes, and --expires-at")
		return 2
	}
	flags := flag.NewFlagSet("token create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	state := flags.String("state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "State directory")
	name := flags.String("name", "", "Token name")
	scopes := flags.String("scopes", "", "Comma-separated scopes")
	expiry := flags.String("expires-at", "", "Expiry in RFC 3339 format")
	if err := flags.Parse(args[2:]); err != nil {
		return 2
	}
	expires, err := time.Parse(time.RFC3339, *expiry)
	if err != nil || flags.NArg() != 0 || *name == "" || *scopes == "" {
		_, _ = fmt.Fprintln(stderr, "set --name, --scopes, and a future --expires-at; use flags only")
		return 2
	}
	store, err := owner.Open(*state)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer func() { _ = store.Close() }()
	grant, err := store.CreateToken(owner.TokenRequest{Name: *name, Scopes: strings.Split(*scopes, ","), ExpiresAt: expires}, time.Now())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(grant); err != nil {
		return 1
	}
	return 0
}
