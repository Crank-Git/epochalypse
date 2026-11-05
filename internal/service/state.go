package service

import (
	"errors"
	"net/http"
	"time"

	"github.com/Crank-Git/epochalypse/internal/authority"
	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/resolver"
	"github.com/Crank-Git/epochalypse/internal/window"
)

type mainState struct {
	owner              *owner.Store
	content            *installation.Store
	capsules           *capsule.Store
	capsuleSlots       chan struct{}
	countdowns         *countdown.Store
	countdownSigner    *countdown.Signer
	countdownAuthority *authority.Authority
	countdownSlots     chan struct{}
	windows            *window.Store
	authority          *authority.Authority
	jobs               *resolver.Jobs
	directAddress      string
	directSlots        chan struct{}
}

func openMainState(config Config) (*mainState, error) {
	owners, err := owner.Open(config.StateDir)
	if err != nil {
		return nil, err
	}
	state := &mainState{owner: owners, directSlots: make(chan struct{}, 4), capsuleSlots: make(chan struct{}, 4)}
	state.content, err = installation.Open(config.StateDir, config.windowLimits())
	if err != nil {
		state.close()
		return nil, err
	}
	state.capsules, err = capsule.Attach(state.content, config.capsuleLimit())
	if err != nil {
		state.close()
		return nil, err
	}
	if err := state.attachCountdowns(config); err != nil {
		state.close()
		return nil, err
	}
	if config.Zone == "" {
		return state, nil
	}
	signer, err := window.LoadSigner(config.Zone, config.KeyFile)
	if err != nil {
		state.close()
		return nil, err
	}
	if state.countdownSigner != nil && signer.PublicKey().PublicKey == state.countdownSigner.PublicKey().PublicKey {
		state.close()
		return nil, errors.New("countdowns require signing material separate from windows")
	}
	if signer.LabOnly() && !config.labMaterialAllowed() {
		state.close()
		return nil, errors.New("production authority rejects an explicit lab key")
	}
	state.windows, err = window.Attach(state.content, signer)
	if err != nil {
		state.close()
		return nil, err
	}
	state.authority, err = authority.New(state.windows, signer, config.nameServer())
	if err != nil {
		state.close()
		return nil, err
	}
	if err := state.windows.RecoverRuns(time.Now()); err != nil {
		state.close()
		return nil, err
	}
	if len(config.Resolvers) > 0 {
		state.jobs, err = resolver.NewJobs(state.windows, config.Resolvers)
		if err != nil {
			state.close()
			return nil, err
		}
	}
	return state, nil
}

func (s *mainState) close() {
	if s.jobs != nil {
		s.jobs.Close()
	}
	if s.windows != nil {
		_ = s.windows.Close()
	}
	if s.capsules != nil {
		_ = s.capsules.Close()
	}
	if s.countdowns != nil {
		_ = s.countdowns.Close()
	}
	if s.content != nil {
		_ = s.content.Close()
	}
	_ = s.owner.Close()
}

func (s *mainState) routes(mux *http.ServeMux, lab bool) {
	api := &capsuleAPI{store: s.capsules, now: func() time.Time { return time.Now().UTC().Truncate(time.Second) }, slots: s.capsuleSlots, timeout: 10 * time.Second, beacon: capsule.QuicknetRelay()}
	api.routes(mux, s.owner, lab)
	countdowns := &countdownAPI{store: s.countdowns, now: time.Now, slots: s.countdownSlots}
	countdowns.routes(mux, s.owner, lab)
	s.publicWindowRoutes(mux, lab)
	s.observationRoutes(mux, lab)
	mux.Handle("/api/v1/windows", owner.SecureHandler(lab, s.owner.RequireOwner(windowHandler(s.windows, time.Now))))
	ownerHandler := s.owner.Handler(lab)
	for _, path := range []string{"/owner", "/owner/", "/api/v1/csrf", "/api/v1/session", "/api/v1/tokens", "/api/v1/tokens/", "/api/v1/dashboard"} {
		mux.Handle(path, ownerHandler)
	}
}

func (s *mainState) dnsResponder() func([]byte, bool) []byte {
	if s.authority == nil && s.countdownAuthority == nil {
		return nil
	}
	return func(data []byte, udp bool) []byte {
		now := time.Now()
		if s.authority != nil {
			response := s.authority.Packet(data, udp, now)
			if s.countdownAuthority == nil || len(response) < 4 || response[3]&15 != 5 {
				return response
			}
		}
		return s.countdownAuthority.Packet(data, udp, now)
	}
}
