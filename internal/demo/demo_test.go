package demo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateRejectsExistingFixtures(t *testing.T) {
	dir := t.TempDir()
	if err := Create(dir); err != nil {
		t.Fatal(err)
	}
	if err := Create(dir); err == nil {
		t.Fatal("existing fixtures replaced")
	}
}

func TestCreateRejectsInvalidDirectories(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Create(file); err == nil {
		t.Fatal("file accepted as state directory")
	}
	if err := Create(filepath.Join(file, "child")); err == nil {
		t.Fatal("invalid parent accepted")
	}
	state := filepath.Join(dir, "state")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "demo"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Create(state); err == nil {
		t.Fatal("file accepted as namespace")
	}
}

func TestDemoRejectsSymbolicLinkEscape(t *testing.T) {
	state, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "keep")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(state, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := Create(state); err == nil {
		t.Fatal("external symbolic link accepted")
	}
	if err := Clean(state, "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("cleanup escapes namespace: %v", err)
	}
	if err := Clean(filepath.Join(state, "missing"), "demo"); err == nil {
		t.Fatal("missing state accepted")
	}
}

func TestCreateRejectsLocalSymbolicLink(t *testing.T) {
	state := t.TempDir()
	if err := os.Mkdir(filepath.Join(state, "other"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other", filepath.Join(state, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := Create(state); err == nil {
		t.Fatal("local symbolic link accepted")
	}
}
