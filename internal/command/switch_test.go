package command

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSwitchCommandCreatesAndReadsDraft(t *testing.T) {
	state := t.TempDir()
	var out, diagnostic bytes.Buffer
	if code := Run([]string{"switch", "create", "--state-dir", state}, &out, &diagnostic); code != 0 {
		t.Fatalf("create exit=%d: %s", code, &diagnostic)
	}
	var created struct {
		Version         int    `json:"version"`
		ID              string `json:"id"`
		IntervalSeconds int64  `json:"interval_seconds"`
		GraceSeconds    int64  `json:"grace_seconds"`
		State           string `json:"state"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Version != 1 || created.ID == "" || created.IntervalSeconds != 86400 || created.GraceSeconds != 3600 || created.State != "draft" {
		t.Fatalf("draft metadata=%+v", created)
	}
	want := out.String()
	out.Reset()
	if code := Run([]string{"switch", "status", "--state-dir", state, "--id", created.ID}, &out, &diagnostic); code != 0 || out.String() != want {
		t.Fatalf("status exit=%d: %s %s", code, &out, &diagnostic)
	}
}

func TestSwitchCommandUsesConfiguredDurations(t *testing.T) {
	var out, diagnostic bytes.Buffer
	args := []string{"switch", "create", "--state-dir", t.TempDir(), "--interval-seconds", "7200", "--grace-seconds", "120"}
	if code := Run(args, &out, &diagnostic); code != 0 {
		t.Fatalf("create exit=%d: %s", code, &diagnostic)
	}
	var record struct {
		IntervalSeconds int64 `json:"interval_seconds"`
		GraceSeconds    int64 `json:"grace_seconds"`
	}
	if err := json.Unmarshal(out.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.IntervalSeconds != 7200 || record.GraceSeconds != 120 {
		t.Fatalf("durations=%+v", record)
	}
}

func TestSwitchCommandRejectsInvalidArguments(t *testing.T) {
	state := t.TempDir()
	for _, args := range [][]string{
		{"switch"},
		{"switch", "arm"},
		{"switch", "create", "extra"},
		{"switch", "create", "--unknown"},
		{"switch", "status"},
		{"switch", "status", "--id", ""},
		{"switch", "create", "--interval-seconds", "invalid"},
	} {
		var out, diagnostic bytes.Buffer
		args = append(args, "--state-dir", state)
		if code := Run(args, &out, &diagnostic); code != 2 || diagnostic.Len() == 0 || out.Len() != 0 {
			t.Fatalf("invalid invocation %q: %d %s %s", args, code, &out, &diagnostic)
		}
	}
}

func TestSwitchCommandReportsDomainAndOutputFailures(t *testing.T) {
	for _, args := range [][]string{
		{"switch", "create", "--state-dir", t.TempDir(), "--interval-seconds", "3599"},
		{"switch", "create", "--state-dir", t.TempDir(), "--grace-seconds", "2592001"},
		{"switch", "status", "--state-dir", t.TempDir(), "--id", "00000000000000000000000000000000"},
	} {
		var out, diagnostic bytes.Buffer
		if code := Run(args, &out, &diagnostic); code != 1 || diagnostic.Len() == 0 || out.Len() != 0 {
			t.Fatalf("failing invocation %q: %d %s %s", args, code, &out, &diagnostic)
		}
	}
	if code := Run([]string{"switch", "create", "--state-dir", t.TempDir()}, failedWriter{}, &bytes.Buffer{}); code != 1 {
		t.Fatalf("output failure succeeds: %d", code)
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostic bytes.Buffer
	if code := Run([]string{"switch", "create", "--state-dir", blocked}, &out, &diagnostic); code != 1 || diagnostic.Len() == 0 {
		t.Fatalf("storage failure: %d %s", code, &diagnostic)
	}
}
