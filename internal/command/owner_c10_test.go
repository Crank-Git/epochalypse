package command

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/owner"
)

func TestOwnerRejectsEmptyAndOversizedPasswordFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "empty"},
		{name: "oversized", data: bytes.Repeat([]byte("x"), 1025)},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			passwordPath := filepath.Join(directory, "password")
			if err := os.WriteFile(passwordPath, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			args := []string{"owner", "init", "--state-dir", filepath.Join(directory, "state"), "--password-file", passwordPath}
			if code := Run(args, &stdout, &stderr); code != 1 {
				t.Fatalf("owner init exits %d", code)
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "password requires 1 to 1024 bytes") {
				t.Fatalf("unexpected output: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			store, err := owner.Open(filepath.Join(directory, "state"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			if err := store.Verify([]byte("synthetic-recovery-check")); !errors.Is(err, owner.ErrCredentials) {
				t.Fatalf("rejected password created an owner identity: %v", err)
			}
		})
	}
}

func TestReadLimitedFileRejectsInvalidPathsAndOversizedInput(t *testing.T) {
	directory := t.TempDir()
	missing := filepath.Join(directory, "missing")
	if _, err := readBoundedFile(missing, 4); !os.IsNotExist(err) {
		t.Fatalf("missing input error = %v", err)
	}
	if _, err := readBoundedFile(directory, 4); err == nil || err.Error() != "input must be an owner-only regular file" {
		t.Fatalf("directory input error = %v", err)
	}
	path := filepath.Join(directory, "large")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedFile(path, 4); err == nil || err.Error() != "input file exceeds limit" {
		t.Fatalf("oversized input error = %v", err)
	}
}

func TestReadOwnerOnlyFileRejectsPublicInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "public")
	if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil { // #nosec G302 -- The test creates a public synthetic input.
		t.Fatal(err)
	}
	if _, err := readOwnerOnlyFile(path, 32); err == nil || err.Error() != "input must be an owner-only regular file" {
		t.Fatalf("public input error = %v", err)
	}
}

func TestReadPrivatePlanRejectsUnknownAndTrailingData(t *testing.T) {
	for _, data := range []string{`{"unexpected":true}`, `{} {}`} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "plan")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readPrivatePlan(path); err == nil || err.Error() != "invalid private switch plan" {
				t.Fatalf("invalid plan error = %v", err)
			}
		})
	}
}

func TestReadEndpointPinsRejectsInvalidMaps(t *testing.T) {
	for _, data := range []string{"{}", `{"https://agent":"bad"}`, `{"":"` + strings.Repeat("a", 64) + `"}`, `{"https://agent":"` + strings.Repeat("a", 62) + `"}`, `{"https://agent":"` + strings.Repeat("a", 64) + `"} {}`} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pins")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readEndpointPins(path); err == nil || err.Error() != "invalid TLS pin map" {
				t.Fatalf("invalid pin map error = %v", err)
			}
		})
	}
}
