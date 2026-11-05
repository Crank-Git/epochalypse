package command

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestCountdownCommandPreservesAbsoluteEventAfterRestart(t *testing.T) {
	state, key := t.TempDir(), windowKey(t)
	event := time.Now().UTC().Truncate(time.Second).Add(time.Hour).Format(time.RFC3339)
	common := []string{"--state-dir", state, "--zone", "windows.example.", "--key-file", key}
	args := append([]string{"countdown", "create", "--title", "Synthetic event", "--event-at", event}, common...)
	var out, diagnostic bytes.Buffer
	if code := Run(args, &out, &diagnostic); code != 0 {
		t.Fatalf("create exit=%d: %s", code, &diagnostic)
	}
	var created struct {
		ID      string `json:"id"`
		EventAt string `json:"event_at"`
		Name    string `json:"name"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.EventAt != event || created.Name != "c"+created.ID+".windows.example." {
		t.Fatalf("metadata=%+v", created)
	}
	want := out.String()
	out.Reset()
	args = append([]string{"countdown", "inspect", "--id", created.ID}, common...)
	if code := Run(args, &out, &diagnostic); code != 0 || out.String() != want {
		t.Fatalf("inspect exit=%d: %s %s", code, &out, &diagnostic)
	}
}
