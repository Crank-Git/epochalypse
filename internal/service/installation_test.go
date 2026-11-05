package service

import (
	"testing"

	"github.com/Crank-Git/epochalypse/internal/installation"
)

func TestMainStateOwnsContentLifetimeWithoutDNS(t *testing.T) {
	directory := t.TempDir()
	state, err := openMainState(Config{StateDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.close)
	other, err := installation.Open(directory, installation.DefaultLimits())
	if err == nil {
		_ = other.Close()
		t.Fatal("a server without DNS permits a second content writer")
	}
	state.close()
	reopened, err := installation.Open(directory, installation.DefaultLimits())
	if err != nil {
		t.Fatalf("shutdown retains the content lock: %v", err)
	}
	_ = reopened.Close()
}
