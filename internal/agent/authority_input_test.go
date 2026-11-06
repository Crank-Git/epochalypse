package agent

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthorityRejectsInvalidInitializationInputs(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"no volume":        func(c *Config) { c.Directory = "" },
		"no checkpoint":    func(c *Config) { c.CheckpointDirectory = "" },
		"no identity":      func(c *Config) { c.ExpectedVolumeID = "" },
		"long identity":    func(c *Config) { c.ExpectedVolumeID = strings.Repeat("x", 257) },
		"newline identity": func(c *Config) { c.ExpectedVolumeID = "volume\nother" },
		"small record":     func(c *Config) { c.MaxRecordBytes = 100 },
		"large record":     func(c *Config) { c.MaxRecordBytes = authorityMaxAllowed + 1 },
		"missing volume":   func(c *Config) { c.Directory = filepath.Join(c.Directory, "absent") },
		"unsafe volume": func(c *Config) {
			if err := os.Chmod(c.Directory, 0o755); err != nil { // #nosec G302 -- The test requires an unsafe authority volume.
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			config := authorityTestConfig(t)
			change(&config)
			if err := Initialize(config); err == nil {
				t.Fatal("invalid configuration initializes authority")
			}
			journal, err := Open(config)
			if journal != nil {
				_ = journal.Close()
			}
			if err == nil {
				t.Fatal("invalid configuration opens authority")
			}
		})
	}
}

func TestAuthorityRejectsCheckpointDamageWithoutRepair(t *testing.T) {
	for name, damage := range map[string]func(*testing.T, string){
		"malformed": func(t *testing.T, path string) { writeDamagedCheckpoint(t, path, []byte("{")) },
		"trailing":  func(t *testing.T, path string) { writeDamagedCheckpoint(t, path, []byte("{} {}")) },
		"checksum":  func(t *testing.T, path string) { writeDamagedCheckpoint(t, path, []byte(`{"version":1}`)) },
		"oversized": func(t *testing.T, path string) { writeDamagedCheckpoint(t, path, bytes.Repeat([]byte("x"), 2049)) },
		"public": func(t *testing.T, path string) {
			if err := os.Chmod(path, 0o644); err != nil { // #nosec G302 -- The test requires a public checkpoint.
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			config := authorityTestConfig(t)
			config.MaxRecordBytes = 2048
			if err := Initialize(config); err != nil {
				t.Fatal(err)
			}
			journal, err := Open(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := journal.Commit(authorityTestState()); err != nil {
				t.Fatal(err)
			}
			if err := journal.Close(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(config.CheckpointDirectory, authorityCheckpointName)
			damage(t, path)
			before, err := os.ReadFile(path) // #nosec G304 -- The test controls the temporary checkpoint path.
			if err != nil {
				t.Fatal(err)
			}
			journal, err = Open(config)
			if journal != nil {
				_ = journal.Close()
			}
			if !errors.Is(err, ErrQuarantined) {
				t.Fatalf("damaged checkpoint error = %v", err)
			}
			after, err := os.ReadFile(path) // #nosec G304 -- The test controls the temporary checkpoint path.
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("open replaces damaged checkpoint: %v", err)
			}
		})
	}
}

func writeDamagedCheckpoint(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
