package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnerCreatesOneIdentity(t *testing.T) {
	state := t.TempDir()
	password := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(password, []byte("synthetic-password-13\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"owner", "init", "--state-dir", state, "--password-file", password}
	var out, diagnostic bytes.Buffer
	if code := Run(args, &out, &diagnostic); code != 0 {
		t.Fatalf("owner init exits %d: %s", code, &diagnostic)
	}
	if !strings.Contains(out.String(), "Owner created") {
		t.Fatalf("missing setup result: %s", &out)
	}
	out.Reset()
	diagnostic.Reset()
	if code := Run(args, &out, &diagnostic); code != 1 {
		t.Fatalf("replacement exits %d", code)
	}
	if !strings.Contains(diagnostic.String(), "owner already exists") {
		t.Fatalf("missing conflict: %s", &diagnostic)
	}
	if strings.Contains(out.String()+diagnostic.String(), "synthetic-password-13") {
		t.Fatal("output exposes password")
	}
}

func TestOwnerRejectsInvalidArgumentsAndPasswordFiles(t *testing.T) {
	for _, args := range [][]string{{"owner"}, {"owner", "unknown"}, {"owner", "init", "--unknown"}, {"owner", "init"}, {"owner", "init", "extra"}} {
		var output bytes.Buffer
		if code := Run(args, &output, &output); code != 2 {
			t.Fatalf("%v exits %d", args, code)
		}
	}
	directory := t.TempDir()
	password := filepath.Join(directory, "password")
	// The fixture verifies that the command rejects public password files.
	if err := os.WriteFile(password, []byte("synthetic"), 0644); err != nil { // #nosec G306
		t.Fatal(err)
	}
	for _, path := range []string{password, directory, filepath.Join(directory, "missing")} {
		var output bytes.Buffer
		if code := Run([]string{"owner", "init", "--state-dir", directory, "--password-file", path}, &output, &output); code != 1 {
			t.Fatal(code)
		}
		if strings.Contains(output.String(), "synthetic") {
			t.Fatal("error contains password")
		}
	}
}

func TestOwnerRecoversThePassword(t *testing.T) {
	directory := t.TempDir()
	password := filepath.Join(directory, "password")
	if err := os.WriteFile(password, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	args := []string{"owner", "init", "--state-dir", directory, "--password-file", password}
	if code := Run(args, &output, &output); code != 0 {
		t.Fatal(output.String())
	}
	args[1] = "recover"
	if code := Run(args, &output, &output); code != 0 || !strings.Contains(output.String(), "Owner recovered; sessions revoked") {
		t.Fatal(output.String())
	}
	if code := Run(args, failedWriter{}, &output); code != 1 {
		t.Fatal("output failure succeeds")
	}
	if err := os.WriteFile(filepath.Join(directory, "bad-state"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	args[3] = filepath.Join(directory, "bad-state")
	if code := Run(args, &output, &output); code != 1 {
		t.Fatal("invalid state succeeds")
	}
}
