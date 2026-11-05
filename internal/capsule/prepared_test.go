package capsule

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
)

func TestPreparedImportPreservesLocalTimelockWithoutContentKey(t *testing.T) {
	local := capsuleStore(t, 32)
	record, err := local.Create(capsuleInput(Tlock), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	remote := capsuleStore(t, 32)
	imported, err := remote.ImportPrepared(bytes.NewReader(data), capsuleNow)
	if err != nil || imported.ID != record.ID || imported.Timelock.Round != record.Timelock.Round {
		t.Fatalf("import: %+v %v", imported, err)
	}
	if _, err := remote.Publish(record.ID, capsuleNow); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Bundle(record.ID, capsuleNow); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.ImportPrepared(bytes.NewReader(data), capsuleNow); err == nil {
		t.Fatal("duplicate overwrites a published record")
	}
	if records, keys := capsuleCounts(t, remote); records != 1 || keys != 1 {
		t.Fatal("duplicate changes retained state")
	}
}

func TestPreparedImportRejectsUntrustedFieldsAndBounds(t *testing.T) {
	local := capsuleStore(t, 32)
	record, err := local.Create(capsuleInput(Tlock), strings.NewReader("private text"), capsuleNow)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(record)
	for _, change := range []func(Record) Record{
		func(r Record) Record { r.Mode = ServerHeld; return r },
		func(r Record) Record { r.PublishedAt = &capsuleNow; return r },
		func(r Record) Record { r.Timelock = nil; return r },
		func(r Record) Record { r.ObjectSHA256 = strings.Repeat("0", 64); return r },
		func(r Record) Record { r.CreatedAt = capsuleNow.Add(time.Second); return r },
	} {
		remote := capsuleStore(t, 32)
		bad, _ := json.Marshal(change(record))
		if _, err := remote.ImportPrepared(bytes.NewReader(bad), capsuleNow); err == nil {
			t.Fatal("accepts invalid prepared state")
		}
		if records, keys := capsuleCounts(t, remote); records != 0 || keys != 0 {
			t.Fatal("failed import retains state")
		}
	}
	for _, bad := range [][]byte{append(data, '\n'), append([]byte(`{"released_key":"secret",`), data[1:]...), append([]byte(`{"version":1,`), data[1:]...), []byte(strings.Repeat("x", 20000))} {
		if _, err := local.ImportPrepared(bytes.NewReader(bad), capsuleNow); err == nil {
			t.Fatal("accepts noncanonical or excessive input")
		}
	}
	remote := capsuleStore(t, 4)
	if _, err := remote.ImportPrepared(bytes.NewReader(data), capsuleNow); !errors.Is(err, ErrLimit) {
		t.Fatalf("plaintext allowance: %v", err)
	}
	quota, err := Open(t.TempDir(), installation.Limits{MaxObjects: 1, MaxBytes: 32}, 32)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = quota.Close() }()
	if _, err := quota.ImportPrepared(bytes.NewReader(data), capsuleNow); !errors.Is(err, installation.ErrQuota) {
		t.Fatalf("quota: %v", err)
	}
	if records, keys := capsuleCounts(t, quota); records != 0 || keys != 0 {
		t.Fatal("quota rejection retains partial state")
	}
}
