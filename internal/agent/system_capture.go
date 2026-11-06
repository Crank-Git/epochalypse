package agent

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type systemClockRead func(int32) (time.Duration, error)

type systemCaptureConfig struct {
	readClock    systemClockRead
	readBootID   func() (string, error)
	wallNow      func() time.Time
	processID    int
	processEpoch string
}

type systemCapture struct {
	config          systemCaptureConfig
	mu              sync.Mutex
	bootID          string
	startBoot       time.Duration
	lastBoot        time.Duration
	lastMono        time.Duration
	lastWall        time.Time
	epochGeneration uint64
	epoch           string
	ready           bool
}

type systemClockSample struct {
	beforeBoot  time.Duration
	beforeMono  time.Duration
	bootID      string
	wall        time.Time
	afterBoot   time.Duration
	afterMono   time.Duration
	afterBootID string
}

func newSystemCapture(config systemCaptureConfig) (func() (Capture, error), error) {
	if config.readClock == nil || config.readBootID == nil || config.wallNow == nil || config.processID < 1 || config.processEpoch == "" {
		return nil, ErrClock
	}
	provider := &systemCapture{config: config, epochGeneration: 1}
	if err := provider.setBaseline(); err != nil {
		return nil, err
	}
	return provider.read, nil
}

func (c *systemCapture) read() (Capture, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ready {
		if err := c.setBaseline(); err != nil {
			return Capture{}, err
		}
	}
	sample, err := c.readSample()
	if err != nil {
		return Capture{}, errors.Join(ErrClock, err, c.reset())
	}
	if !c.valid(sample) {
		if sample.afterBootID == "" || sample.afterBoot < 0 || sample.afterMono < 0 {
			return Capture{}, errors.Join(ErrClock, c.reset())
		}
		c.setEpoch(sample.afterBootID, sample.afterBoot, sample.afterMono, sample.wall)
		return Capture{}, ErrClock
	}
	c.lastBoot, c.lastMono, c.lastWall = sample.afterBoot, sample.afterMono, sample.wall
	return Capture{
		Epoch: c.epoch, UTCSeconds: sample.wall.Unix(),
		CapturedElapsed: sample.beforeBoot - c.startBoot,
		DecisionElapsed: sample.afterBoot - c.startBoot,
	}, nil
}

func (c *systemCapture) readSample() (systemClockSample, error) {
	var sample systemClockSample
	var err error
	sample.beforeBoot, err = c.config.readClock(clockBoottime)
	if err != nil {
		return sample, err
	}
	sample.beforeMono, err = c.config.readClock(clockMonotonic)
	if err != nil {
		return sample, err
	}
	sample.bootID, err = c.config.readBootID()
	if err != nil {
		return sample, err
	}
	sample.wall = c.config.wallNow().UTC().Round(0)
	sample.afterBoot, err = c.config.readClock(clockBoottime)
	if err != nil {
		return sample, err
	}
	sample.afterMono, err = c.config.readClock(clockMonotonic)
	if err != nil {
		return sample, err
	}
	sample.afterBootID, err = c.config.readBootID()
	return sample, err
}

func (c *systemCapture) valid(sample systemClockSample) bool {
	if sample.beforeBoot < 0 || sample.beforeMono < 0 || sample.afterBoot < 0 || sample.afterMono < 0 {
		return false
	}
	if sample.bootID == "" || sample.afterBootID == "" || sample.bootID != c.bootID || sample.afterBootID != c.bootID ||
		sample.afterBoot < sample.beforeBoot || sample.afterMono < sample.beforeMono {
		return false
	}
	if sample.beforeBoot < c.startBoot || sample.afterBoot-sample.beforeBoot > 250*time.Millisecond {
		return false
	}
	bootDelta, monoDelta := sample.afterBoot-c.lastBoot, sample.afterMono-c.lastMono
	if bootDelta < 0 || monoDelta < 0 || absDuration(bootDelta-monoDelta) > 250*time.Millisecond {
		return false
	}
	return absDuration(sample.wall.Sub(c.lastWall)-bootDelta) <= 250*time.Millisecond
}

func (c *systemCapture) setBaseline() error {
	bootID, err := c.config.readBootID()
	if err != nil || bootID == "" {
		return errors.Join(ErrClock, err)
	}
	boot, err := c.config.readClock(clockBoottime)
	if err != nil || boot < 0 {
		return errors.Join(ErrClock, err)
	}
	mono, err := c.config.readClock(clockMonotonic)
	if err != nil || mono < 0 {
		return errors.Join(ErrClock, err)
	}
	c.setEpoch(bootID, boot, mono, c.config.wallNow())
	c.ready = true
	return nil
}

func (c *systemCapture) reset() error {
	c.ready = false
	return c.setBaseline()
}

func (c *systemCapture) setEpoch(bootID string, boot, mono time.Duration, wall time.Time) {
	c.epochGeneration++
	c.bootID = bootID
	c.startBoot, c.lastBoot = boot, boot
	c.lastMono = mono
	c.lastWall = wall.UTC().Round(0)
	c.epoch = makeEpoch(bootID, c.config.processID, c.config.processEpoch, c.epochGeneration)
	c.ready = true
}

func makeEpoch(bootID string, processID int, processEpoch string, generation uint64) string {
	return fmt.Sprintf("%s:%d:%s:%d", bootID, processID, processEpoch, generation)
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		if value == time.Duration(-1<<63) {
			return time.Duration(1<<63 - 1)
		}
		return -value
	}
	return value
}
