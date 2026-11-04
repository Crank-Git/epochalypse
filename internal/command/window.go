package command

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/Crank-Git/epochalypse/internal/window"
)

func runWindow(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[1] != "create" && args[1] != "inspect") {
		_, _ = fmt.Fprintln(stderr, "use window create or window inspect with --zone and --key-file; stop the main server first")
		return 2
	}
	flags := flag.NewFlagSet("window "+args[1], flag.ContinueOnError)
	flags.SetOutput(stderr)
	state := flags.String("state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "State directory; stop the main server first")
	zone := flags.String("zone", "", "Dedicated experimental zone")
	keyFile := flags.String("key-file", os.Getenv("EPOCHALYPSE_KEY_FILE"), "Protected BIND private key file")
	maxObjects := flags.String("max-objects", envDefault("EPOCHALYPSE_MAX_OBJECTS", "100"), "Maximum retained window documents")
	maxBytes := flags.String("quota-bytes", envDefault("EPOCHALYPSE_QUOTA_BYTES", "1073741824"), "Maximum retained window JSON bytes")
	var input window.Input
	id := ""
	if args[1] == "create" {
		flags.StringVar(&input.Content, "content", "", "Public TXT content, at most 512 UTF-8 bytes")
		flags.StringVar(&input.Inception, "inception", "", "Explicit inception in whole UTC seconds")
		flags.StringVar(&input.Expiration, "expiration", "", "Explicit expiration in whole UTC seconds")
	} else {
		flags.StringVar(&id, "id", "", "Public window identifier")
	}
	if err := flags.Parse(args[2:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *zone == "" || *keyFile == "" || (args[1] == "inspect" && id == "") {
		_, _ = fmt.Fprintln(stderr, "set --zone, --key-file, and the window fields; use flags only")
		return 2
	}
	limits, err := parseWindowLimits(*maxObjects, *maxBytes)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	record, err := executeWindow(*state, *zone, *keyFile, id, input, limits)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(record); err != nil {
		return 1
	}
	return 0
}

func executeWindow(state, zone, keyFile, id string, input window.Input, limits window.Limits) (window.Window, error) {
	signer, err := window.LoadSigner(zone, keyFile)
	if err != nil {
		return window.Window{}, err
	}
	store, err := window.OpenWithLimits(state, signer, limits)
	if err != nil {
		return window.Window{}, err
	}
	defer func() { _ = store.Close() }()
	if id != "" {
		return store.Get(id)
	}
	return store.Create(input, time.Now())
}

func parseWindowLimits(objects, bytes string) (window.Limits, error) {
	count, countErr := strconv.Atoi(objects)
	size, sizeErr := strconv.ParseInt(bytes, 10, 64)
	limits := window.Limits{MaxObjects: count, MaxBytes: size}
	if countErr != nil || sizeErr != nil || limits.Validate() != nil {
		return window.Limits{}, fmt.Errorf("set positive integer retention limits")
	}
	return limits, nil
}
