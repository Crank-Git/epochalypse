package agent

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAuthorityCapacityRefusalPreservesAcceptedStateAcrossRestart(t *testing.T) {
	config := authorityTestConfig(t)
	config.MaxRecordBytes = 1024
	if err := Initialize(config); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()
	accepted := authorityTestState()
	accepted.PossibleDisclosure, accepted.UnsafeClock = true, true
	if _, err := journal.Commit(accepted); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config.Directory, authorityJournalName)
	before, err := os.ReadFile(path) // #nosec G304 -- The test reads its temporary authority journal.
	if err != nil {
		t.Fatal(err)
	}
	next := cloneAuthorityState(accepted)
	next.Generation++
	next.Ciphertext = make([]byte, 2048)
	if _, err := journal.Commit(next); !errors.Is(err, ErrRecordLimit) {
		t.Fatalf("capacity error=%v", err)
	}
	after, err := os.ReadFile(path) // #nosec G304 -- The test reads its temporary authority journal.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("capacity refusal changes accepted journal bytes")
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	retained, err := reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retained, accepted) {
		t.Fatal("capacity refusal changes accepted state after restart")
	}
}
