package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func TestConfigRejectsIncompleteWindowSettings(t *testing.T) {
	for _, config := range []Config{
		{Zone: "windows.example."},
		{KeyFile: "missing.private"},
		{Zone: "windows.example.", KeyFile: "missing.private"},
		{Mode: "dns", Zone: "windows.example.", KeyFile: "missing.private", DNSAddr: "127.0.0.1:0"},
		{WindowLimits: &window.Limits{}},
	} {
		if config.Mode == "" {
			config.Mode = "server"
		}
		config.HTTPAddr, config.StateDir = "127.0.0.1:0", t.TempDir()
		if err := config.Validate(); err == nil {
			t.Fatal("invalid window settings pass")
		}
	}
}

func TestMainStateReleasesOwnerAfterWindowFailure(t *testing.T) {
	for _, keyFailure := range []bool{true, false} {
		state := t.TempDir()
		config := Config{StateDir: state, Zone: "windows.example.", KeyFile: serviceKey(t)}
		if keyFailure {
			config.KeyFile = "missing.private"
		} else {
			if err := os.MkdirAll(filepath.Join(state, "v1", "windows.db"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		if s, err := openMainState(config); err == nil {
			s.close()
			t.Fatal("invalid window state opens")
		}
		owners, err := owner.Open(state)
		if err != nil {
			t.Fatal("failed startup retains the owner lock")
		}
		if err := owners.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadSettingsIncludesTheZone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"zone":"windows.example."}`), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := ReadSettings(path)
	if err != nil || settings.Zone != "windows.example." || settings.Version != 1 {
		t.Fatalf("settings: %+v %v", settings, err)
	}
}
