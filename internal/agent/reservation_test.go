package agent

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestJournalCapacityLeavesFinalRecordsAfterAdmissionRefusal(t *testing.T) {
	config := authorityTestConfig(t)
	config.MaxRecordBytes = 1024
	config.MaxJournalBytes = 8 << 10
	config.MaxJournalRecords = 6
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	j, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	state := authorityTestState()
	for state.Generation <= 4 {
		if _, err := j.Commit(state); err != nil {
			t.Fatal(err)
		}
		state.Generation++
	}
	path := filepath.Join(config.Directory, authorityJournalName)
	before, err := os.ReadFile(path) // #nosec G304 -- The test owns this temporary journal path.
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Commit(state); err == nil {
		t.Fatal("admission consumes final record capacity")
	}
	after, err := os.ReadFile(path) // #nosec G304 -- The test owns this temporary journal path.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("capacity refusal changes journal")
	}
	state.UnsafeClock = true
	if _, err := j.Commit(state); err != nil {
		t.Fatal(err)
	}
	state.Generation++
	state.PossibleDisclosure = true
	if _, err := j.Commit(state); err != nil {
		t.Fatal(err)
	}
	assertFinalCapacityStateAfterRestart(t, j, config)
}

func assertFinalCapacityStateAfterRestart(t *testing.T, j *Journal, config Config) {
	t.Helper()
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	got, err := reopened.Snapshot()
	if err != nil || !got.UnsafeClock || !got.PossibleDisclosure || got.Generation != 6 {
		t.Fatalf("retained state=%+v error=%v", got, err)
	}
}

func TestRenewalCapacityRefusalPrecedesAcceptedDeadlineMutation(t *testing.T) {
	f, tr, next := armedCheckinFixture(t)
	before, err := f.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	f.journal.config.MaxJournalRecords = 7
	request, err := switchwire.SignFrame(next, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Checkin(request); !errors.Is(err, ErrCapacity) {
		t.Fatalf("renewal capacity error=%v", err)
	}
	after, err := f.journal.Snapshot()
	if err != nil || !equalAuthorityState(before, after) {
		t.Fatal("known capacity refusal changes accepted deadline")
	}
}

func TestCheckpointParentSyncFailurePreventsReadiness(t *testing.T) {
	config := authorityTestConfig(t)
	config.CheckpointDirectory = filepath.Join(t.TempDir(), "new-parent", "checkpoint")
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	fault := errors.New("checkpoint parent sync failure")
	config.performIO = func(operation string, action func() error) error {
		if operation == "sync-checkpoint-parent" {
			return fault
		}
		return action()
	}
	j, err := Open(config)
	if j != nil {
		_ = j.Close()
	}
	if !errors.Is(err, fault) {
		t.Fatalf("checkpoint parent sync error=%v", err)
	}
}

func TestJournalByteCapacityPreservesAcceptedState(t *testing.T) {
	config := authorityTestConfig(t)
	config.MaxRecordBytes, config.MaxJournalBytes = 1024, 8192
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	j, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	state := authorityTestState()
	for attempts := 0; attempts < 30; attempts++ {
		_, err = j.Commit(state)
		if errors.Is(err, ErrCapacity) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		state.Generation++
	}
	if !errors.Is(err, ErrCapacity) {
		t.Fatal("journal byte admission is unbounded")
	}
	retained, err := j.Snapshot()
	if err != nil || retained.Generation != state.Generation-1 {
		t.Fatal("byte capacity changes accepted state")
	}
	state.PossibleDisclosure = true
	if _, err := j.Commit(state); err != nil {
		t.Fatalf("admission consumes release bytes: %v", err)
	}
}

func TestSyntheticStorageRequiresExplicitLabClock(t *testing.T) {
	config := ServiceConfig{Authority: authorityTestConfig(t), VerifiedClock: &ChronyClock{}}
	if service, err := OpenService(config); err == nil || service != nil {
		t.Fatal("verified clock accepts synthetic storage")
	}
}

type blockedPublicWriter struct {
	header           http.Header
	entered, unblock chan struct{}
}

func (w *blockedPublicWriter) Header() http.Header { return w.header }
func (w *blockedPublicWriter) WriteHeader(_ int)   {}
func (w *blockedPublicWriter) Write(data []byte) (int, error) {
	close(w.entered)
	<-w.unblock
	return len(data), nil
}

func TestSlowPublicOutputPreservesPublisherAccessAndBoundsRequests(t *testing.T) {
	_, tr, _ := armedCheckinFixture(t)
	tr.clock = func() (Reading, error) { return Reading{lower: 4660}, nil }
	handler := tr.Handler()
	writers := make([]*blockedPublicWriter, 2)
	for i := range writers {
		writers[i] = &blockedPublicWriter{header: make(http.Header), entered: make(chan struct{}), unblock: make(chan struct{})}
		writer := writers[i]
		defer close(writer.unblock)
		go handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/agent/v1/public/switches/switch-1/ciphertext", nil))
		select {
		case <-writer.entered:
		case <-time.After(3 * time.Second):
			t.Fatal("public output does not start")
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/agent/v1/public/switches/switch-1", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("request capacity status=%d", w.Code)
	}
	done := make(chan error, 1)
	go func() { done <- tr.Publish() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("slow output blocks publication")
	}
}

func TestCheckpointRetainsBothReservedInodesAcrossCommits(t *testing.T) {
	config := authorityTestConfig(t)
	config.MaxRecordBytes = 1024
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	j, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	active := filepath.Join(config.CheckpointDirectory, authorityCheckpointName)
	spare := filepath.Join(config.CheckpointDirectory, "checkpoint.spare")
	a, err := os.Stat(active)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(spare)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Commit(authorityTestState()); err != nil {
		t.Fatal(err)
	}
	c, err := os.Stat(active)
	if err != nil {
		t.Fatal(err)
	}
	d, err := os.Stat(spare)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(a, d) || !os.SameFile(b, c) {
		t.Fatal("checkpoint replacement discards reserved inodes")
	}
	if c.Size() != 1024 {
		t.Fatalf("checkpoint size=%d", c.Size())
	}
}
