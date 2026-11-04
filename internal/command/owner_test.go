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
