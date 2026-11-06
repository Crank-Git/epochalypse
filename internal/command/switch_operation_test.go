package command

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSwitchArmRejectsMalformedPrivateInputs(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.json")
	key := filepath.Join(dir, "owner.key")
	pins := filepath.Join(dir, "pins.json")
	for path, value := range map[string]string{plan: "{}", key: "not base64", pins: "{}"} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"switch", "arm", "--state-dir", filepath.Join(dir, "state"), "--plan", plan, "--owner-key", key, "--bundle", filepath.Join(dir, "bundle"), "--pins", pins}
	var out, diagnostic bytes.Buffer
	if code := Run(args, &out, &diagnostic); code != 1 || out.Len() != 0 || diagnostic.Len() == 0 {
		t.Fatalf("invalid arm inputs: exit=%d stdout=%q stderr=%q", code, out.String(), diagnostic.String())
	}
	if bytes.Contains(diagnostic.Bytes(), []byte("not base64")) {
		t.Fatalf("diagnostic exposed private input: %s", diagnostic.String())
	}
}

func TestSwitchArmRequiresOwnerOnlyKeyFile(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(plan, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "owner.key")
	if err := os.WriteFile(key, []byte("invalid key"), 0644); err != nil { // #nosec G306 -- The test requires a group-readable key fixture.
		t.Fatal(err)
	}
	args := []string{"switch", "arm", "--state-dir", filepath.Join(dir, "state"), "--plan", plan, "--owner-key", key, "--bundle", filepath.Join(dir, "bundle"), "--pins", filepath.Join(dir, "missing-pins")}
	var out, diagnostic bytes.Buffer
	if code := Run(args, &out, &diagnostic); code != 1 || out.Len() != 0 || !bytes.Contains(diagnostic.Bytes(), []byte("owner-only")) {
		t.Fatalf("unprotected key: exit=%d stdout=%q stderr=%q", code, out.String(), diagnostic.String())
	}
}
