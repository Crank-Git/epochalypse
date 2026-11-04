package service

import (
	"net/http"
	"time"

	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/window"
)

type mainState struct {
	owner   *owner.Store
	windows *window.Store
}

func openMainState(config Config) (*mainState, error) {
	owners, err := owner.Open(config.StateDir)
	if err != nil {
		return nil, err
	}
	state := &mainState{owner: owners}
	if config.Zone == "" {
		return state, nil
	}
	signer, err := window.LoadSigner(config.Zone, config.KeyFile)
	if err != nil {
		state.close()
		return nil, err
	}
	state.windows, err = window.OpenWithLimits(config.StateDir, signer, config.windowLimits())
	if err != nil {
		state.close()
		return nil, err
	}
	return state, nil
}

func (s *mainState) close() {
	if s.windows != nil {
		_ = s.windows.Close()
	}
	_ = s.owner.Close()
}

func (s *mainState) routes(mux *http.ServeMux, lab bool) {
	mux.Handle("/api/v1/windows", owner.SecureHandler(lab, s.owner.RequireOwner(windowHandler(s.windows, time.Now))))
	ownerHandler := s.owner.Handler(lab)
	for _, path := range []string{"/owner", "/owner/", "/api/v1/csrf", "/api/v1/session", "/api/v1/tokens", "/api/v1/tokens/", "/api/v1/dashboard"} {
		mux.Handle(path, ownerHandler)
	}
}
