package window

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Crank-Git/epochalypse/internal/observation"
	bolt "go.etcd.io/bbolt"
	"reflect"
	"strings"
	"time"
)

var observationBucket = []byte("observations-v1")

// ErrRunCompleted rejects a second completion of an immutable run.
var ErrRunCompleted = errors.New("observation run is complete")

const runReservation = 16384

type runDocument struct {
	Run           observation.Run `json:"run"`
	ReservedBytes int             `json:"reserved_bytes"`
}

// ReserveRun reserves the shared installation budget before a resolver job starts.
func (s *Store) ReserveRun(id string, targets []observation.Target, now time.Time) (observation.Run, error) {
	if err := observation.ValidateTargets(targets); err != nil {
		return observation.Run{}, err
	}
	if now.IsZero() {
		return observation.Run{}, errors.New("provide the run UTC time")
	}
	w, err := s.Get(id)
	if err != nil {
		return observation.Run{}, err
	}
	var random [16]byte
	_, _ = rand.Read(random[:])
	run := observation.Run{Version: 1, ID: hex.EncodeToString(random[:]), WindowID: id, Name: w.Name, State: "pending", Started: now.UTC(), Results: make([]observation.Result, len(targets))}
	for i, target := range targets {
		run.Results[i] = observation.Result{Version: 1, Name: w.Name, Resolver: target}
	}
	data, err := json.Marshal(runDocument{run, runReservation})
	if err != nil {
		return observation.Run{}, fmt.Errorf("encode pending observation: %w", err)
	}
	if len(data) > runReservation-2048 {
		return observation.Run{}, errors.New("resolver metadata exceeds the observation bound")
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		if err := s.limits.admits(tx, runReservation); err != nil {
			return err
		}
		return tx.Bucket(observationBucket).Put([]byte(id+"/"+run.ID), data)
	})
	if err != nil {
		return observation.Run{}, fmt.Errorf("reserve observation: %w", err)
	}
	// Decode the committed bytes so caller-owned metadata cannot change the result.
	doc, _ := decodeRun(data)
	return doc.Run, nil
}

func decodeRun(data []byte) (runDocument, error) {
	if data == nil {
		return runDocument{}, ErrNotFound
	}
	var doc runDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return runDocument{}, errors.New("invalid durable observation document")
	}
	if doc.Run.Version != 1 || !validID(doc.Run.ID) || !validID(doc.Run.WindowID) {
		return runDocument{}, errors.New("invalid durable observation identity")
	}
	if len(doc.Run.Results) == 0 || len(doc.Run.Results) > 3 || doc.Run.Started.IsZero() {
		return runDocument{}, errors.New("invalid durable observation interval")
	}
	if doc.Run.State == "pending" {
		if doc.ReservedBytes != runReservation || doc.Run.Completed != nil {
			return runDocument{}, errors.New("invalid durable observation reservation")
		}
	} else if (doc.Run.State != "complete" && doc.Run.State != "partial") || doc.ReservedBytes != 0 || doc.Run.Completed == nil {
		return runDocument{}, errors.New("invalid durable observation state")
	}
	return doc, nil
}

func observationBytes(data []byte) (int, error) {
	doc, err := decodeRun(data)
	if err != nil {
		return 0, err
	}
	return max(len(data), doc.ReservedBytes), nil
}

// CompleteRun commits all results before it releases the pending byte reservation.
func (s *Store) CompleteRun(windowID, runID string, results []observation.Result, now time.Time) (observation.Run, error) {
	if !validID(windowID) || !validID(runID) {
		return observation.Run{}, errors.New("invalid observation identifier")
	}
	var completed observation.Run
	err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(observationBucket)
		key := []byte(windowID + "/" + runID)
		doc, err := decodeRun(bucket.Get(key))
		if err != nil {
			return err
		}
		if doc.Run.State != "pending" {
			return ErrRunCompleted
		}
		completed, err = finishRun(doc.Run, results, now)
		if err != nil {
			return err
		}
		data, err := json.Marshal(runDocument{Run: completed})
		if err != nil {
			return fmt.Errorf("encode complete observation: %w", err)
		}
		if len(data) > runReservation {
			return errors.New("observation exceeds its byte reservation")
		}
		return bucket.Put(key, data)
	})
	if err != nil {
		return observation.Run{}, fmt.Errorf("complete observation: %w", err)
	}
	return completed, nil
}

func finishRun(run observation.Run, results []observation.Result, now time.Time) (observation.Run, error) {
	if len(results) != len(run.Results) {
		return observation.Run{}, errors.New("provide one result for each configured resolver")
	}
	run.State = "complete"
	for i, result := range results {
		if err := validateResult(run, run.Results[i].Resolver, result); err != nil {
			return observation.Run{}, err
		}
		if result.Error != "" {
			run.State = "partial"
		}
	}
	completed := now.UTC()
	run.Completed = &completed
	run.Results = results
	return run, nil
}

func validateResult(run observation.Run, target observation.Target, result observation.Result) error {
	if result.Version != 1 || result.Name != run.Name || !reflect.DeepEqual(result.Resolver, target) {
		return errors.New("observation differs from its reserved resolver or window")
	}
	if result.Started.IsZero() || result.Completed.IsZero() {
		return errors.New("provide the observed UTC times")
	}
	if result.Error != "" {
		if result.Rcode != nil || result.Flags != nil || (result.Error != "resolver_unavailable" && result.Error != "run_interrupted") {
			return errors.New("an unavailable resolver cannot claim a DNS response")
		}
		return nil
	}
	if result.Rcode == nil || result.Flags == nil || *result.Rcode > 4095 || (result.Transport != "udp" && result.Transport != "tcp") {
		return errors.New("provide the resolver response and transport")
	}
	return nil
}

// ListRuns returns at most fifty runs and a cursor for the next page.
func (s *Store) ListRuns(windowID, after string) ([]observation.Run, string, error) {
	if _, err := s.Get(windowID); err != nil {
		return nil, "", err
	}
	if after != "" && !validID(after) {
		return nil, "", errors.New("invalid observation cursor")
	}
	runs := []observation.Run{}
	next := ""
	err := s.db.View(func(tx *bolt.Tx) error {
		prefix := windowID + "/"
		cursor := tx.Bucket(observationBucket).Cursor()
		for key, data := cursor.Seek([]byte(prefix + after)); key != nil && strings.HasPrefix(string(key), prefix); key, data = cursor.Next() {
			if string(key) == prefix+after {
				continue
			}
			if len(runs) == 50 {
				next = runs[len(runs)-1].ID
				break
			}
			doc, err := decodeRun(data)
			if err != nil {
				return err
			}
			runs = append(runs, doc.Run)
		}
		return nil
	})
	return runs, next, err
}

// RecoverRuns marks pending jobs unavailable after a process restart.
func (s *Store) RecoverRuns(now time.Time) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(observationBucket)
		pending := []runDocument{}
		if err := bucket.ForEach(func(_, data []byte) error {
			doc, err := decodeRun(data)
			if err != nil {
				return err
			}
			if doc.Run.State == "pending" {
				pending = append(pending, doc)
			}
			return nil
		}); err != nil {
			return err
		}
		for _, doc := range pending {
			results := doc.Run.Results
			for i := range results {
				results[i].Started = now.UTC()
				results[i].Completed = now.UTC()
				results[i].Error = "run_interrupted"
			}
			run, err := finishRun(doc.Run, results, now)
			if err != nil {
				return err
			}
			data, err := json.Marshal(runDocument{Run: run})
			if err != nil {
				return fmt.Errorf("encode interrupted observation: %w", err)
			}
			if err := bucket.Put([]byte(run.WindowID+"/"+run.ID), data); err != nil {
				return err
			}
		}
		return nil
	})
}
