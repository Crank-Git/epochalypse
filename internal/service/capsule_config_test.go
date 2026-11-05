package service

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/installation"
)

func TestMainStateMountsCapsuleRoutesWithoutDNS(t *testing.T) {
	h := newCapsuleHTTP(t)
	*h.clock = time.Now().UTC().Truncate(time.Second)
	mux := http.NewServeMux()
	h.state.routes(mux, false)
	h.handler = mux
	createHTTPCapsule(t, h, "server-held")
}

func TestSettingsAcceptConfiguredCapsulePlaintextLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"capsule_plaintext_limit":4}`), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := ReadSettings(path)
	if err != nil {
		t.Fatalf("configured capsule limit fails: %v", err)
	}
	if settings.CapsulePlaintextLimit != 4 {
		t.Fatal("the configured limit changes")
	}
}

func TestCapsuleConfigurationRejectsInvalidLimits(t *testing.T) {
	for _, limit := range []int{-1, int(^uint(0) >> 1)} {
		config := Config{Mode: "server", HTTPAddr: "127.0.0.1:0", StateDir: t.TempDir(), CapsulePlaintextLimit: limit}
		if err := config.Validate(); err == nil {
			t.Fatalf("an invalid capsule limit passes: %d", limit)
		}
		if _, err := openMainState(config); err == nil {
			t.Fatalf("an invalid capsule limit opens state: %d", limit)
		}
		shared, err := installation.Open(config.StateDir, installation.DefaultLimits())
		if err != nil {
			t.Fatalf("failed capsule setup retains the content lock: %v", err)
		}
		_ = shared.Close()
	}
	if (Config{}).capsuleLimit() != capsule.DefaultPlaintextLimit {
		t.Fatal("the default capsule limit changes")
	}
}

func TestMainStateUsesConfiguredCapsuleLimit(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Second)
	config := Config{StateDir: t.TempDir(), CapsulePlaintextLimit: 4}
	state, err := openMainState(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.close)
	if state.capsules.PlaintextLimit() != 4 {
		t.Fatal("the main state omits the configured upload limit")
	}
	if _, err := state.capsules.Create(capsule.Input{Mode: capsule.ServerHeld, Kind: capsule.Text, ReleaseAt: at.Add(time.Hour)}, strings.NewReader("12345"), at); !errors.Is(err, capsule.ErrLimit) {
		t.Fatalf("the configured state admits oversized content: %v", err)
	}
}
