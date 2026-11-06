package command

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadinessCommandRejectsAbsentAndInvalidLocalInventory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"private":"secret-inventory-marker"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"readiness"}, {"readiness", "--bad"}, {"readiness", "--inventory", path, "extra"}, {"readiness", "--inventory", path}, {"readiness", "--inventory", path + "missing"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code == 0 || stdout.Len() != 0 || bytes.Contains(stderr.Bytes(), []byte("secret-inventory-marker")) {
			t.Fatalf("command: %d %s %s", code, &stdout, &stderr)
		}
	}
}
