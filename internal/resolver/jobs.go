package resolver

import (
	"context"
	"errors"
	"github.com/Crank-Git/epochalypse/internal/observation"
	"github.com/Crank-Git/epochalypse/internal/window"
	"sync"
	"time"
)

// ErrBusy identifies the fixed admission bound for resolver work.
var ErrBusy = errors.New("resolver jobs are busy")

// ErrUnavailable identifies a closed or failed resolver job service.
var ErrUnavailable = errors.New("resolver jobs are unavailable")

// Jobs owns bounded resolver work for the main process.
type Jobs struct {
	store   *window.Store
	targets []observation.Target
	ctx     context.Context
	cancel  context.CancelFunc
	slots   chan struct{}
	mu      sync.Mutex
	closed  bool
	failed  bool
	wg      sync.WaitGroup
	query   func(context.Context, string, observation.Target) observation.Result
}

// NewJobs validates trusted targets before the service accepts work.
func NewJobs(store *window.Store, targets []observation.Target) (*Jobs, error) {
	if store == nil {
		return nil, errors.New("configure the window store before resolver jobs")
	}
	if err := observation.ValidateTargets(targets); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	copied := make([]observation.Target, len(targets))
	for i, target := range targets {
		copied[i] = target.Clone()
	}
	return &Jobs{store: store, targets: copied, ctx: ctx, cancel: cancel, slots: make(chan struct{}, 4), query: Observe}, nil
}

// Start reserves durable state before it starts asynchronous resolver work.
func (j *Jobs) Start(id string) (observation.Run, error) {
	j.mu.Lock()
	if j.closed || j.failed {
		j.mu.Unlock()
		return observation.Run{}, ErrUnavailable
	}
	select {
	case j.slots <- struct{}{}:
	default:
		j.mu.Unlock()
		return observation.Run{}, ErrBusy
	}
	j.wg.Add(1)
	j.mu.Unlock()
	run, err := j.store.ReserveRun(id, j.targets, time.Now())
	if err != nil {
		j.release()
		return observation.Run{}, err
	}
	go j.execute(run)
	return run, nil
}

func (j *Jobs) execute(run observation.Run) {
	defer j.release()
	results := make([]observation.Result, len(j.targets))
	for i, target := range j.targets {
		results[i] = j.query(j.ctx, run.Name, target)
	}
	if _, err := j.store.CompleteRun(run.WindowID, run.ID, results, time.Now()); err != nil {
		// A failed durable completion must remain unavailable until recovery.
		j.mu.Lock()
		j.failed = true
		j.cancel()
		j.mu.Unlock()
	}
}

func (j *Jobs) release() { <-j.slots; j.wg.Done() }

// Close cancels queries and drains accepted jobs before store closure.
func (j *Jobs) Close() {
	j.mu.Lock()
	j.closed = true
	j.cancel()
	j.mu.Unlock()
	j.wg.Wait()
}

// Available reports whether the service can accept or expose observation work.
func (j *Jobs) Available() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return !j.closed && !j.failed
}
