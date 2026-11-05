package window

import (
	"encoding/json"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestObservationLoadsRejectMismatchedDurableContext(t *testing.T) {
	changes := map[string]func(*runDocument){
		"run identifier":    func(doc *runDocument) { doc.Run.ID = strings.Repeat("0", 32) },
		"window identifier": func(doc *runDocument) { doc.Run.WindowID = strings.Repeat("0", 32) },
		"window name":       func(doc *runDocument) { doc.Run.Name = "other.windows.example." },
		"result name":       func(doc *runDocument) { doc.Run.Results[0].Name = "other.windows.example." },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			s, w := observationStore(t, DefaultLimits())
			run, err := s.ReserveRun(w.ID, observationTargets(), testNow)
			if err != nil {
				t.Fatal(err)
			}
			key := []byte(w.ID + "/" + run.ID)
			if err := s.db.Update(func(tx *bolt.Tx) error {
				bucket := tx.Bucket(observationBucket)
				doc, err := decodeRun(bucket.Get(key))
				if err != nil {
					return err
				}
				change(&doc)
				data, err := json.Marshal(doc)
				if err != nil {
					return err
				}
				return bucket.Put(key, data)
			}); err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.ListRuns(w.ID, ""); err == nil {
				t.Error("the public read accepts mismatched durable context")
			}
			if _, err := s.CompleteRun(w.ID, run.ID, completeResults(run), testNow); err == nil {
				t.Error("completion accepts mismatched durable context")
			}
			if err := s.RecoverRuns(testNow); err == nil {
				t.Error("recovery accepts mismatched durable context")
			}
			if _, err := s.Create(validInput(), testNow); err == nil {
				t.Error("the shared quota accepts mismatched durable context")
			}
		})
	}
}
