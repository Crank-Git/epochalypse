package main

import (
	"os"

	"github.com/Crank-Git/epochalypse/internal/command"
)

func main() {
	os.Exit(command.Run(os.Args[1:], os.Stdout, os.Stderr))
}
