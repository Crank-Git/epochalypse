package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func runCountdown(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[1] != "create" && args[1] != "inspect") {
		_, _ = fmt.Fprintln(stderr, "use countdown create or countdown inspect; stop the main server first")
		return 2
	}
	flags := flag.NewFlagSet("countdown "+args[1], flag.ContinueOnError)
	flags.SetOutput(stderr)
	state := flags.String("state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "State directory; stop the main server first")
	zone := flags.String("zone", "", "Separate countdown zone")
	key := flags.String("key-file", os.Getenv("EPOCHALYPSE_COUNTDOWN_KEY_FILE"), "Separate protected BIND private key file")
	objects := flags.String("max-objects", envDefault("EPOCHALYPSE_MAX_OBJECTS", "100"), "Maximum retained content objects")
	quota := flags.String("quota-bytes", envDefault("EPOCHALYPSE_QUOTA_BYTES", "1073741824"), "Maximum retained content bytes")
	var input countdown.Input
	id := ""
	if args[1] == "create" {
		flags.StringVar(&input.Title, "title", "", "Public event title")
		flags.StringVar(&input.EventAt, "event-at", "", "Future event in whole UTC seconds")
	} else {
		flags.StringVar(&id, "id", "", "Countdown identifier")
	}
	if err := flags.Parse(args[2:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 || (args[1] == "inspect" && id == "") || (args[1] == "create" && (*zone == "" || *key == "")) {
		_, _ = fmt.Fprintln(stderr, "set the countdown fields; creation requires --zone and --key-file; use flags only")
		return 2
	}
	limits, err := parseWindowLimits(*objects, *quota)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	r, err := executeCountdown(*state, *zone, *key, id, input, limits)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(r); err != nil {
		return 1
	}
	return 0
}

func executeCountdown(directory, zone, key, id string, input countdown.Input, limits window.Limits) (countdown.Record, error) {
	if id == "" {
		if _, err := window.LoadSigner(zone, key); err != nil {
			return countdown.Record{}, err
		}
	}
	s, err := countdown.Open(directory, limits, zone)
	if err != nil {
		return countdown.Record{}, err
	}
	defer func() { _ = s.Close() }()
	if id != "" {
		return s.Get(id)
	}
	return s.Create(context.Background(), input, time.Now())
}
