package command

import (
	"fmt"
	"io"
)

// Run returns the exit code for the command arguments.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 || (len(args) == 1 && args[0] != "help" && args[0] != "-h" && args[0] != "--help") {
		if _, err := fmt.Fprintln(stderr, "invalid command: use epochalypse help"); err != nil {
			return 1
		}
		return 2
	}
	if _, err := fmt.Fprintln(stdout, "Usage: epochalypse [help]\n\nCommands:\n  help  Show command help"); err != nil {
		return 1
	}
	return 0
}
