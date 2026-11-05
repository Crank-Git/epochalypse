package service

import (
	"net/http"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/owner"
)

type capsuleAPI struct {
	store   *capsule.Store
	now     func() time.Time
	slots   chan struct{}
	timeout time.Duration
}

func (api *capsuleAPI) routes(mux *http.ServeMux, owners *owner.Store, lab bool) {}
