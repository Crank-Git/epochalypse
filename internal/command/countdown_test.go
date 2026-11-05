package command

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestCountdownCommandRejectsInvalidArguments(t *testing.T) {
	state, key := t.TempDir(), windowKey(t)
	for _, args := range [][]string{
		{"countdown"},
		{"countdown", "delete"},
		{"countdown", "inspect", "--unknown"},
		{"countdown", "inspect"},
		{"countdown", "create"},
		{"countdown", "inspect", "--id", "invalid", "extra"},
		{"countdown", "create", "--zone", "windows.example.", "--key-file", key, "--max-objects", "invalid"},
	} {
		var out, diagnostic bytes.Buffer
		args = append(args, "--state-dir", state)
		if code := Run(args, &out, &diagnostic); code != 2 || diagnostic.Len() == 0 || out.Len() != 0 {
			t.Fatalf("invalid invocation %q: %d %s %s", args, code, &out, &diagnostic)
		}
	}
}

func TestCountdownCommandReportsKeyStoreAndOutputFailures(t *testing.T) {
	state, key := t.TempDir(), windowKey(t)
	event := time.Now().UTC().Add(time.Hour).Truncate(time.Second).Format(time.RFC3339)
	args := []string{"countdown", "create", "--state-dir", state, "--zone", "windows.example.", "--key-file", key, "--title", "Synthetic event", "--event-at", event}
	var out, diagnostic bytes.Buffer
	if code := Run(args, failedWriter{}, &diagnostic); code != 1 {
		t.Fatalf("output failure succeeds: %d", code)
	}
	for _, extra := range [][]string{
		{"--key-file", filepath.Join(t.TempDir(), "missing.private")},
		{"--event-at", "past"},
	} {
		if code := Run(append(append([]string{}, args...), extra...), &out, &diagnostic); code != 1 {
			t.Fatalf("invalid creation succeeds: %d", code)
		}
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := Run(append(args, "--state-dir", blocked), &out, &diagnostic); code != 1 {
		t.Fatalf("blocked storage succeeds: %d", code)
	}
}
