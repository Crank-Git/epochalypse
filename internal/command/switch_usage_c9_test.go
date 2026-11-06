package command

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSwitchUsageErrorsPreserveStateDirectory(t *testing.T) {
	for _, args := range [][]string{
		{"switch"},
		{"switch", "prepare", "--unknown"},
		{"switch", "prepare"},
		{"switch", "prepare", "unexpected"},
	} {
		t.Run(args[len(args)-1], func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "uncreated-state")
			t.Setenv("EPOCHALYPSE_STATE_DIR", state)
			var stdout, stderr bytes.Buffer
			if code := Run(args, &stdout, &stderr); code != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("usage error: exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if _, err := os.Stat(state); !os.IsNotExist(err) {
				t.Fatalf("usage error opens persistent state: %v", err)
			}
		})
	}
}
