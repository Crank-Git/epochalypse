package agent

import (
	"errors"
	"testing"
	"time"
)

func TestSystemCaptureUsesFreshUTCAndElapsedBounds(t *testing.T) {
	wall := time.Unix(10_000, 0)
	read, err := newSystemCapture(systemCaptureConfig{
		readClock:  advancingSystemClock(),
		readBootID: func() (string, error) { return "00000000-0000-0000-0000-000000000001", nil },
		wallNow: func() time.Time {
			current := wall
			wall = wall.Add(20 * time.Millisecond)
			return current
		},
		processID:    42,
		processEpoch: "process-one",
	})
	if err != nil {
		t.Fatal(err)
	}
	capture, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if capture.UTCSeconds != 10_000 || capture.CapturedElapsed < 0 || capture.DecisionElapsed <= capture.CapturedElapsed {
		t.Fatalf("capture = %+v", capture)
	}
	if capture.Epoch == "" {
		t.Fatal("capture has no process epoch")
	}
}

func TestSystemCaptureRejectsBootIDChange(t *testing.T) {
	ids := []string{"boot-one", "boot-two"}
	index := 0
	read, err := newSystemCapture(systemCaptureConfig{
		readClock: advancingSystemClock(),
		readBootID: func() (string, error) {
			id := ids[min(index, len(ids)-1)]
			index++
			return id, nil
		},
		wallNow:      advancingSystemWall(),
		processID:    42,
		processEpoch: "process-one",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := read(); !errors.Is(err, ErrClock) {
		t.Fatalf("capture error = %v, want ErrClock", err)
	}
	recovered, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Epoch == "" {
		t.Fatal("recovered capture has no assessment epoch")
	}
}

func TestSystemCaptureRejectsSuspendGap(t *testing.T) {
	boot, mono := time.Second, time.Second
	suspended := false
	wallCalls := 0
	read, err := newSystemCapture(systemCaptureConfig{
		readClock: func(clockID int32) (time.Duration, error) {
			if clockID == clockBoottime {
				boot += 10 * time.Millisecond
				return boot, nil
			}
			mono += 10 * time.Millisecond
			if mono > time.Second+10*time.Millisecond {
				mono = time.Second + 10*time.Millisecond
			}
			return mono, nil
		},
		readBootID: func() (string, error) { return "boot-one", nil },
		wallNow: func() time.Time {
			wallCalls++
			if wallCalls == 3 && !suspended {
				boot += time.Second
				suspended = true
			}
			return wallSample()
		},
		processID:    42,
		processEpoch: "process-one",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := read(); !errors.Is(err, ErrClock) {
		t.Fatalf("capture error = %v, want ErrClock", err)
	}
	recovered, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Epoch == first.Epoch {
		t.Fatal("recovered capture kept the suspended assessment epoch")
	}
}

func TestSystemCaptureInvalidatesWallStepAndRecovers(t *testing.T) {
	wall := []time.Time{
		time.Unix(10_000, 0),
		time.Unix(10_000, 20_000_000),
		time.Unix(10_001, 40_000_000),
		time.Unix(10_001, 60_000_000),
	}
	index := 0
	read, err := newSystemCapture(systemCaptureConfig{
		readClock:  advancingSystemClock(),
		readBootID: func() (string, error) { return "boot-one", nil },
		wallNow: func() time.Time {
			value := wall[min(index, len(wall)-1)]
			index++
			return value
		},
		processID:    42,
		processEpoch: "process-one",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := read(); !errors.Is(err, ErrClock) {
		t.Fatalf("capture error = %v, want ErrClock", err)
	}
	recovered, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Epoch == first.Epoch {
		t.Fatal("recovered capture kept the invalid assessment epoch")
	}
}

func TestSystemCaptureInvalidatesBackwardElapsedClockAndRecovers(t *testing.T) {
	boot, mono := time.Second, time.Second
	moveBackward := false
	wallCalls := 0
	triggered := false
	read, err := newSystemCapture(systemCaptureConfig{
		readClock: func(clockID int32) (time.Duration, error) {
			if moveBackward {
				boot -= time.Second
				mono -= time.Second
				moveBackward = false
			}
			if clockID == clockBoottime {
				boot += 10 * time.Millisecond
				return boot, nil
			}
			mono += 10 * time.Millisecond
			return mono, nil
		},
		readBootID: func() (string, error) { return "boot-one", nil },
		wallNow: func() time.Time {
			wallCalls++
			if wallCalls == 3 && !triggered {
				moveBackward = true
				triggered = true
			}
			return wallSample()
		},
		processID:    42,
		processEpoch: "process-one",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := read(); !errors.Is(err, ErrClock) {
		t.Fatalf("capture error = %v, want ErrClock", err)
	}
	recovered, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Epoch == first.Epoch {
		t.Fatal("recovered capture kept the backward-clock assessment epoch")
	}
}

func TestSystemCaptureRejectsIncompleteProvider(t *testing.T) {
	if _, err := newSystemCapture(systemCaptureConfig{}); !errors.Is(err, ErrClock) {
		t.Fatalf("constructor error = %v, want ErrClock", err)
	}
}

func TestSystemCaptureResetsAfterClockReadFailure(t *testing.T) {
	readCount := 0
	failAt := 0
	clock := func(_ int32) (time.Duration, error) {
		readCount++
		if readCount == failAt {
			return 0, errors.New("clock read failed")
		}
		return time.Duration(readCount) * 10 * time.Millisecond, nil
	}
	read, err := newSystemCapture(systemCaptureConfig{
		readClock: clock, readBootID: func() (string, error) { return "boot-one", nil },
		wallNow: advancingSystemWall(), processID: 42, processEpoch: "process-one",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := read()
	if err != nil {
		t.Fatal(err)
	}
	failAt = readCount + 1
	if _, err := read(); !errors.Is(err, ErrClock) {
		t.Fatalf("capture error = %v, want ErrClock", err)
	}
	recovered, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Epoch == first.Epoch {
		t.Fatal("recovered capture kept the failed assessment epoch")
	}
}

func advancingSystemClock() systemClockRead {
	boot, mono := time.Second, time.Second
	return func(clockID int32) (time.Duration, error) {
		if clockID == clockBoottime {
			boot += 10 * time.Millisecond
			return boot, nil
		}
		mono += 10 * time.Millisecond
		return mono, nil
	}
}

func advancingSystemWall() func() time.Time {
	wall := time.Unix(10_000, 0)
	return func() time.Time {
		current := wall
		wall = wall.Add(20 * time.Millisecond)
		return current
	}
}

func wallSample() time.Time { return time.Unix(10_000, 0) }
