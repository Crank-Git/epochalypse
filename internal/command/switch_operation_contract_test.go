package command

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func TestMergeArmReceiptsPreservesPriorProofWithoutDuplicates(t *testing.T) {
	prior := switchwire.Envelope{Body: []byte("prior body"), Signature: []byte("prior signature")}
	current := switchwire.Envelope{Body: []byte("current body"), Signature: []byte("current signature")}
	evidence := &switches.ArmEvidence{Armed: []switchwire.Envelope{prior, current}}
	merged := mergeArmReceipts([]switchwire.Envelope{current}, evidence)
	if len(merged) != 2 || !bytes.Equal(merged[0].Body, current.Body) || !bytes.Equal(merged[1].Body, prior.Body) {
		t.Fatalf("merged receipts lose prior proof or change order: %#v", merged)
	}
	merged = mergeArmReceipts([]switchwire.Envelope{prior}, evidence)
	if len(merged) != 2 {
		t.Fatalf("duplicate receipt is retained: %#v", merged)
	}
	if got := mergeArmReceipts([]switchwire.Envelope{prior}, nil); len(got) != 1 {
		t.Fatalf("receipt changes without prior evidence: %#v", got)
	}
}

func TestSwitchPrepareRejectsMissingOrNonRegularContent(t *testing.T) {
	for _, name := range []string{"missing", "directory"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			draft, err := executeSwitch(dir, "", switches.Input{}, window.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			_, key, err := ed25519.GenerateKey(nil)
			if err != nil {
				t.Fatal(err)
			}
			keyPath := filepath.Join(dir, "owner.key")
			if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
				t.Fatal(err)
			}
			agentsPath := filepath.Join(dir, "agents.json")
			agents, err := json.Marshal(prepareCommandAgents(t))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(agentsPath, agents, 0o600); err != nil {
				t.Fatal(err)
			}
			contentPath := filepath.Join(dir, "content")
			if name == "directory" {
				if err := os.Mkdir(contentPath, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			var stdout, stderr bytes.Buffer
			args := []string{"switch", "prepare", "--state-dir", dir, "--id", draft.ID, "--content", contentPath, "--agents", agentsPath, "--owner-key", keyPath, "--plan", filepath.Join(dir, "plan")}
			if code := Run(args, &stdout, &stderr); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "switch content") {
				t.Fatalf("invalid content exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}
