package backup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/switches"
	bolt "go.etcd.io/bbolt"
)

func TestInstallValidatesSwitchRecordsBeforePublication(t *testing.T) {
	for _, name := range []string{"valid", "unknown field", "wrong key", "unknown version", "invalid state", "nested bucket", "trailing value"} {
		t.Run(name, func(t *testing.T) {
			a := installationArchive(t, func(root string) {
				db, err := bolt.Open(filepath.Join(root, "state/v1/windows.db"), 0600, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = db.Close() }()
				if err := db.Update(func(tx *bolt.Tx) error { return putSwitchSnapshotRecord(tx, name) }); err != nil {
					t.Fatal(err)
				}
			})
			destination := restoreDestination(t, a)
			err := a.Install(context.Background(), destination, testLimits(t))
			if name == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("restore accepts invalid switch state")
			}
			if strings.Contains(err.Error(), "synthetic private value") {
				t.Fatal("restore error exposes retained state")
			}
			if _, err := os.Lstat(destination.Path); !os.IsNotExist(err) {
				t.Fatal("restore publishes invalid switch state")
			}
		})
	}
}

func putSwitchSnapshotRecord(tx *bolt.Tx, name string) error {
	bucket, err := tx.CreateBucketIfNotExists([]byte(installation.SwitchesBucket))
	if err != nil {
		return err
	}
	record := switches.Record{Version: 1, ID: strings.Repeat("a", 32), IntervalSeconds: 86400, GraceSeconds: 3600, CreatedAt: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC), State: switches.StateDraft}
	if name == "unknown version" {
		record.Version = 2
	}
	if name == "invalid state" {
		record.State = "armed"
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if name == "unknown field" {
		data = append(data[:len(data)-1], []byte(`,"private_key":"synthetic private value"}`)...)
	}
	if name == "trailing value" {
		data = append(data, []byte(` {}`)...)
	}
	key := record.ID
	if name == "wrong key" {
		key = strings.Repeat("b", 32)
	}
	if name == "nested bucket" {
		_, err := bucket.CreateBucket([]byte(key))
		return err
	}
	return bucket.Put([]byte(key), data)
}
