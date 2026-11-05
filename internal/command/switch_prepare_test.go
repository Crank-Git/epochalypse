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

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func TestSwitchPrepareCreatesProtectedSignedPlanWithoutPlaintextOutput(t *testing.T) {
	dir := t.TempDir()
	draft, err := executeSwitch(dir, "", switches.Input{}, window.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	keyPath, agentsPath, contentPath, planPath := filepath.Join(dir, "owner.key"), filepath.Join(dir, "agents.json"), filepath.Join(dir, "content"), filepath.Join(dir, "plan")
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(key)), 0600); err != nil {
		t.Fatal(err)
	}
	agents := prepareCommandAgents(t)
	data, err := json.Marshal(agents)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentsPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	secret := []byte("synthetic private switch content")
	if err := os.WriteFile(contentPath, secret, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"switch", "prepare", "--state-dir", dir, "--id", draft.ID, "--content", contentPath, "--agents", agentsPath, "--owner-key", keyPath, "--plan", planPath}
	if code := runSwitch(args, &stdout, &stderr); code != 0 {
		t.Fatalf("prepare=%d: %s", code, &stderr)
	}
	plan, err := readPrivatePlan(planPath)
	if err != nil || validateArmPlan(draft.ID, plan, key) != nil {
		t.Fatalf("retained plan: %v", err)
	}
	if bytes.Contains(stdout.Bytes(), secret) || bytes.Contains(stderr.Bytes(), secret) || bytes.Contains(plan.Ciphertext, secret) {
		t.Fatal("prepare exposes plaintext")
	}
	if code := runSwitch(args, &stdout, &stderr); code == 0 {
		t.Fatal("prepare replaces an existing private plan")
	}
}

func TestSwitchArmRejectsUnstableOrOversizedOwnerFiles(t *testing.T) {
	dir := t.TempDir()
	for _, test := range []struct {
		name string
		mode os.FileMode
		data []byte
	}{
		{name: "directory", mode: 0o700},
		{name: "group readable", mode: 0o640, data: []byte("key")},
		{name: "oversized", mode: 0o600, data: bytes.Repeat([]byte("x"), 5)},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(test.name, " ", "-"))
			if test.name == "directory" {
				if err := os.Mkdir(path, test.mode); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, test.data, test.mode); err != nil {
				t.Fatal(err)
			}
			if _, err := readOwnerOnlyFile(path, 4); err == nil {
				t.Fatal("invalid owner file passes")
			}
		})
	}
}

func prepareCommandAgents(t *testing.T) [3]switchwire.Agent {
	t.Helper()
	agents := [3]switchwire.Agent{}
	for i, id := range []string{"agent1", "agent2", "agent3"} {
		public, _, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		agents[i] = switchwire.Agent{ID: id, Endpoint: "https://" + id + ".invalid", Recipient: identity.Recipient().String(), SigningKey: [32]byte(public)}
	}
	return agents
}

func TestSwitchPrepareRejectsMalformedEnrollment(t *testing.T) {
	dir := t.TempDir()
	draft, err := executeSwitch(dir, "", switches.Input{}, window.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	keyPath, agentsPath := filepath.Join(dir, "owner.key"), filepath.Join(dir, "agents.json")
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentsPath, []byte(`[{"id":"agent1"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	contentPath := filepath.Join(dir, "content")
	if err := os.WriteFile(contentPath, []byte("synthetic content"), 0o600); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(dir, "plan")
	err = prepareSwitch(switchPrepareOptions{state: dir, id: draft.ID, content: contentPath, agents: agentsPath, key: keyPath, plan: planPath})
	if err == nil || !strings.Contains(err.Error(), "exactly three agents") {
		t.Fatalf("malformed enrollment error = %v", err)
	}
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Fatalf("malformed enrollment created a plan: %v", err)
	}
}
