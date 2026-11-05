package switchclient

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

func TestArmRetryRequiresRecoveryFileDurability(t *testing.T) {
	for _, operation := range []string{"file-sync", "directory-sync"} {
		for _, suffix := range []string{".intent", "", ".arm"} {
			t.Run(operation+suffix, func(t *testing.T) {
				p, key, replicas := armFixture(t)
				path := replicas.bundle + suffix
				cause := errors.New("synthetic persistent sync failure")
				originalFile, originalDirectory := syncBundleFile, syncDirectory
				t.Cleanup(func() { syncBundleFile, syncDirectory = originalFile, originalDirectory })
				faulty := true
				syncBundleFile = func(got string, file *os.File) error {
					if faulty && operation == "file-sync" && got == path {
						return cause
					}
					return originalFile(got, file)
				}
				syncDirectory = func(got string, directory *os.File) error {
					if faulty && operation == "directory-sync" && got == path {
						return cause
					}
					return originalDirectory(got, directory)
				}

				for attempt := range 2 {
					result, err := PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle)
					if !errors.Is(err, cause) || result.Confirmed || replicas.armCalls != 0 {
						t.Fatalf("attempt %d bypasses durability failure: confirmed=%t sends=%d error=%v", attempt, result.Confirmed, replicas.armCalls, err)
					}
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				faulty = false
				result, err := PrepareAndArm(context.Background(), p, key, replicas, replicas.bundle)
				if err != nil || !result.Confirmed || replicas.armCalls != 3 {
					t.Fatalf("durable retry fails: confirmed=%t sends=%d error=%v", result.Confirmed, replicas.armCalls, err)
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("durable retry changes retained bytes: %v", err)
				}
			})
		}
	}
}
