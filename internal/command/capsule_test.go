package command

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/capsule"
)

func TestCapsuleCommandsCreateAndPublishMetadata(t *testing.T) {
	directory := t.TempDir()
	content := filepath.Join(directory, "content.txt")
	if err := os.WriteFile(content, []byte("private synthetic content"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"public", "server-held", "tlock"} {
		var out, errors bytes.Buffer
		args := []string{"capsule", "create", "--state-dir", directory, "--mode", mode, "--content-file", content, "--release-at", "2099-01-01T00:00:00Z"}
		if code := Run(args, &out, &errors); code != 0 {
			t.Fatalf("create: %d %s", code, &errors)
		}
		var draft capsule.Manifest
		if err := json.Unmarshal(out.Bytes(), &draft); err != nil {
			t.Fatal(err)
		}
		if draft.PublishedAt != nil || draft.Mode != capsule.Mode(mode) || strings.Contains(out.String(), "private synthetic content") || strings.Contains(out.String(), `"object"`) || strings.Contains(out.String(), `"key"`) {
			t.Fatal(out.String())
		}
		out.Reset()
		if code := Run([]string{"capsule", "publish", "--state-dir", directory, "--id", draft.ID}, &out, &errors); code != 0 {
			t.Fatalf("publish: %d %s", code, &errors)
		}
		var published capsule.Manifest
		if err := json.Unmarshal(out.Bytes(), &published); err != nil {
			t.Fatal(err)
		}
		if published.PublishedAt == nil || published.ID != draft.ID {
			t.Fatal(out.String())
		}
	}
}

func TestCapsuleCommandsRejectInvalidInputWithoutContent(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "content")
	if err := os.WriteFile(path, []byte("private synthetic content"), 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"capsule", "create", "--state-dir", directory, "--content-file", path, "--mode", "server-held", "--release-at", "2099-01-01T00:00:00Z"}
	for _, args := range [][]string{
		{"capsule"}, {"capsule", "open"}, {"capsule", "create", "--bad"}, {"capsule", "create"},
		{"capsule", "publish"}, {"capsule", "publish", "--id", "bad"},
		append(append([]string{}, base...), "extra"),
		append(append([]string{}, base...), "--plaintext-limit", "0"),
		append(append([]string{}, base...), "--plaintext-limit", "2"),
		append(append([]string{}, base...), "--max-objects", "bad"),
		append(append([]string{}, base...), "--release-at", "bad"),
		append(append([]string{}, base...), "--release-at", "2099-01-01T00:00:00.1Z"),
		append(append([]string{}, base...), "--mode", "unknown"),
		append(append([]string{}, base...), "--content-file", directory),
		append(append([]string{}, base...), "--content-file", path+"missing"),
	} {
		var out, errors bytes.Buffer
		if code := Run(args, &out, &errors); code == 0 {
			t.Fatalf("accepts %v", args)
		}
		if out.Len() != 0 || strings.Contains(errors.String(), "private synthetic content") {
			t.Fatalf("discloses content: %s %s", &out, &errors)
		}
	}
}

func TestCapsuleCommandWritesPreparedTimelockWithoutPlaintext(t *testing.T) {
	directory := t.TempDir()
	content, prepared := filepath.Join(directory, "content.txt"), filepath.Join(directory, "prepared.json")
	if err := os.WriteFile(content, []byte("private synthetic content"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"capsule", "create", "--state-dir", directory, "--mode", "tlock", "--content-file", content, "--release-at", "2099-01-01T00:00:00Z", "--prepared-output", prepared}
	var out, errors bytes.Buffer
	if code := Run(args, &out, &errors); code != 0 {
		t.Fatalf("prepare: %d %s", code, &errors)
	}
	data, err := os.ReadFile(prepared)
	if err != nil {
		t.Fatal(err)
	}
	var record capsule.Record
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.Mode != capsule.Tlock || record.Timelock == nil || record.PublishedAt != nil || bytes.Contains(data, []byte("private synthetic content")) || bytes.Contains(data, []byte(`"key"`)) {
		t.Fatal("prepared output violates its trust boundary")
	}
	info, err := os.Stat(prepared)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("prepared output lacks private permissions")
	}
	out.Reset()
	if code := Run(args, &out, &errors); code == 0 {
		t.Fatal("prepared output overwrites an existing file")
	}
	if retained, err := os.ReadFile(prepared); err != nil || !bytes.Equal(data, retained) {
		t.Fatal("failed preparation changes existing output")
	}
}
