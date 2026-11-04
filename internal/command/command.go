package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/Crank-Git/epochalypse/internal/service"
)

// Run returns the exit code for the command arguments.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "owner":
			return runOwner(args, stdout, stderr)
		case "token":
			return runToken(args, stdout, stderr)
		case "window":
			return runWindow(args, stdout, stderr)
		case "status", "demo", "clean-demo":
			return runFoundation(args, stdout, stderr)
		case "server", "dns", "agent":
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runService(ctx, args, stderr)
		}
	}
	if len(args) > 1 || (len(args) == 1 && args[0] != "help" && args[0] != "-h" && args[0] != "--help") {
		if _, err := fmt.Fprintln(stderr, "invalid command: use epochalypse help"); err != nil {
			return 1
		}
		return 2
	}
	if _, err := fmt.Fprintln(stdout, "Usage: epochalypse <command> [flags]\n\nCommands:\n  help        Show command help\n  owner       Create or recover the local owner\n  token       Create a scoped token with host access\n  window      Create or inspect signed windows offline\n  server      Start the web server\n  dns         Start the local DNS scaffold\n  agent       Start a local agent scaffold\n  status      Verify a deployed commit\n  demo        Create synthetic fixtures\n  clean-demo  Remove the explicit demo namespace"); err != nil {
		return 1
	}
	return 0
}

func envDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func runService(ctx context.Context, args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", envDefault("EPOCHALYPSE_CONFIG", "config/local.json"), "Path to the version 1 local configuration")
	config := service.Config{Mode: args[0]}
	flags.StringVar(&config.HTTPAddr, "http-addr", envDefault("EPOCHALYPSE_HTTP_ADDR", "127.0.0.1:8080"), "HTTP listen address")
	flags.StringVar(&config.DNSAddr, "dns-addr", envDefault("EPOCHALYPSE_DNS_ADDR", "127.0.0.1:1053"), "TCP and UDP DNS listen address")
	flags.StringVar(&config.StateDir, "state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "State directory")
	flags.StringVar(&config.AgentID, "agent-id", os.Getenv("EPOCHALYPSE_AGENT_ID"), "Local agent identity")
	flags.BoolVar(&config.LocalLab, "local-lab", false, "Permit owner HTTP access on a loopback address")
	flags.StringVar(&config.TLSCert, "tls-cert", "", "TLS certificate file")
	flags.StringVar(&config.TLSKey, "tls-key", "", "TLS private key file")
	flags.StringVar(&config.Zone, "zone", "", "Dedicated experimental zone")
	flags.StringVar(&config.KeyFile, "key-file", os.Getenv("EPOCHALYPSE_KEY_FILE"), "Protected BIND private key file")
	maxObjects := flags.String("max-objects", envDefault("EPOCHALYPSE_MAX_OBJECTS", "100"), "Maximum retained window documents")
	maxBytes := flags.String("quota-bytes", envDefault("EPOCHALYPSE_QUOTA_BYTES", "1073741824"), "Maximum retained window JSON bytes")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "service commands accept flags only")
		return 2
	}
	limits, err := parseWindowLimits(*maxObjects, *maxBytes)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	config.WindowLimits = &limits
	settings, err := service.ReadSettings(*configPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if config.Zone == "" {
		config.Zone = settings.Zone
	}
	if err := service.Serve(ctx, config, func(service.Addresses) {}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
