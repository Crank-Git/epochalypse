package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func runSwitch(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[1] != "create" && args[1] != "status") {
		_, _ = fmt.Fprintln(stderr, "use switch create or switch status; these commands keep the switch unarmed")
		return 2
	}
	flags := flag.NewFlagSet("switch "+args[1], flag.ContinueOnError)
	flags.SetOutput(stderr)
	state := flags.String("state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "State directory; stop the main server first")
	var id string
	intervalSeconds := switches.DefaultIntervalSeconds
	graceSeconds := switches.DefaultGraceSeconds
	if args[1] == "create" {
		flags.Int64Var(&intervalSeconds, "interval-seconds", intervalSeconds, "Check-in interval in seconds")
		flags.Int64Var(&graceSeconds, "grace-seconds", graceSeconds, "Grace after the deadline in seconds")
	} else {
		flags.StringVar(&id, "id", "", "Switch identifier")
	}
	if err := flags.Parse(args[2:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 || (args[1] == "status" && id == "") {
		_, _ = fmt.Fprintln(stderr, "provide switch fields with flags only")
		return 2
	}
	input := switches.Input{}
	if args[1] == "create" {
		input.IntervalSeconds = &intervalSeconds
		input.GraceSeconds = &graceSeconds
	}
	record, err := executeSwitch(*state, id, input, window.DefaultLimits())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(record); err != nil {
		return 1
	}
	return 0
}

func executeSwitch(directory, id string, input switches.Input, limits window.Limits) (switches.Record, error) {
	db, err := installation.Open(directory, installation.Limits(limits))
	if err != nil {
		return switches.Record{}, err
	}
	defer func() { _ = db.Close() }()
	store, err := switches.Attach(db)
	if err != nil {
		return switches.Record{}, err
	}
	defer func() { _ = store.Close() }()
	if id != "" {
		return store.Get(id)
	}
	return store.Create(context.Background(), input, time.Now())
}
