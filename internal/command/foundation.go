package command

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Crank-Git/epochalypse/internal/demo"
	"github.com/Crank-Git/epochalypse/internal/status"
)

func runFoundation(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	if args[0] == "status" {
		return runStatus(flags, args[1:], stdout, stderr)
	}
	state := flags.String("state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "State directory")
	namespace := flags.String("namespace", "", "Explicit demo namespace for cleanup")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "demo commands accept flags only")
		return 2
	}
	var err error
	if args[0] == "demo" {
		err = demo.Create(*state)
	} else {
		err = demo.Clean(*state, *namespace)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, "synthetic demo namespace:", *state+"/demo"); err != nil {
		return 1
	}
	return 0
}

func runStatus(flags *flag.FlagSet, args []string, stdout, stderr io.Writer) int {
	endpoints := flags.String("endpoints", envDefault("EPOCHALYPSE_STATUS_ENDPOINTS", "http://127.0.0.1:8080"), "Comma-separated service origins")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || !status.ValidCommit(flags.Arg(0)) {
		_, _ = fmt.Fprintln(stderr, "status requires one full lowercase commit SHA")
		return 2
	}
	state, commit := status.Check(strings.Split(*endpoints, ","), flags.Arg(0))
	if _, err := fmt.Fprintf(stdout, "%s local %s\n", state, commit); err != nil {
		return 1
	}
	return 0
}
