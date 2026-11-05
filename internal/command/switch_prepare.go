package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchclient"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
	"github.com/Crank-Git/epochalypse/internal/window"
)

type switchPrepareOptions struct {
	state, id, content, agents, key, plan string
}

func runSwitchPrepare(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("switch prepare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	options := switchPrepareOptions{}
	flags.StringVar(&options.state, "state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "State directory; stop the main server first")
	flags.StringVar(&options.id, "id", "", "Retained draft identifier")
	flags.StringVar(&options.content, "content", "", "Plaintext content file; at most 10 MiB")
	flags.StringVar(&options.agents, "agents", "", "JSON array of three operator-pinned agents")
	flags.StringVar(&options.key, "owner-key", "", "Owner-only base64 Ed25519 private key file")
	flags.StringVar(&options.plan, "plan", "", "New owner-only signed plan path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || options.id == "" || options.content == "" || options.agents == "" || options.key == "" || options.plan == "" {
		_, _ = fmt.Fprintln(stderr, "provide --id, --content, --agents, --owner-key, and --plan")
		return 2
	}
	if err := prepareSwitch(options); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "private switch plan saved; no agent request is sent")
	return 0
}

func prepareSwitch(options switchPrepareOptions) error {
	draft, err := executeSwitch(options.state, options.id, switches.Input{}, window.DefaultLimits())
	if err != nil {
		return err
	}
	if draft.State != "draft" {
		return errors.New("prepare requires a retained draft without an issued arm request")
	}
	key, err := readOwnerPrivateKey(options.key)
	if err != nil {
		return err
	}
	agents, err := readSwitchAgents(options.agents)
	if err != nil {
		return err
	}
	file, err := os.Open(options.content) // #nosec G304 -- The owner selects the local content file.
	if err != nil {
		return errors.New("cannot open switch content")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("switch content requires a regular file")
	}
	material, err := switchcontent.Prepare(file, [3]string{agents[0].Recipient, agents[1].Recipient, agents[2].Recipient})
	if err != nil {
		return err
	}
	plan, err := switchclient.NewPlan(draft, material, agents, key, time.Now())
	if err != nil {
		return err
	}
	return switchclient.WritePlan(options.plan, plan, key)
}

func readSwitchAgents(path string) ([3]switchwire.Agent, error) {
	data, err := readBoundedFile(path, 32<<10)
	if err != nil {
		return [3]switchwire.Agent{}, errors.New("cannot read agent enrollment")
	}
	var agents []switchwire.Agent
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&agents) != nil || decoder.Decode(&struct{}{}) != io.EOF || len(agents) != 3 {
		return [3]switchwire.Agent{}, errors.New("agent enrollment requires exactly three agents")
	}
	return [3]switchwire.Agent(agents), nil
}
