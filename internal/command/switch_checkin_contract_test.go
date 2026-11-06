package command

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchclient"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestSwitchCheckinRejectsInvalidOwnerInputs(t *testing.T) {
	for _, input := range []string{"plan", "owner key", "pins"} {
		t.Run(input, func(t *testing.T) {
			o, _, _ := armCommandFixture(t)
			path := o.planPath
			switch input {
			case "owner key":
				path = o.keyPath
			case "pins":
				path = o.pinsPath
			}
			if input == "pins" {
				writeFixtureFile(t, path, []byte(`{"http://invalid.example":"bad"}`), 0o600)
			} else {
				writeFixtureFile(t, path, []byte("invalid owner input"), 0o600)
			}
			args := []string{"switch", "checkin", "--plan", o.planPath, "--owner-key", o.keyPath, "--pins", o.pinsPath, "--journal", filepath.Join(t.TempDir(), "journal")}
			var stdout, stderr bytes.Buffer
			if code := Run(args, &stdout, &stderr); code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("invalid %s: code=%d stdout=%q stderr=%q", input, code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestSwitchCheckinRejectsFlagErrors(t *testing.T) {
	for _, args := range [][]string{{"switch", "checkin", "--unknown"}, {"switch", "checkin", "--plan", "x", "--owner-key", "y", "--pins", "z", "--journal", "j", "extra"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("invalid flags %v: code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestSwitchJournalStatusReportsMissingOrInvalidOwnerInputs(t *testing.T) {
	o, _, _ := armCommandFixture(t)
	args := []string{"switch", "status", "--plan", o.planPath, "--owner-key", o.keyPath, "--journal", filepath.Join(t.TempDir(), "missing-journal")}
	var stdout, stderr bytes.Buffer
	if code := Run(args, &stdout, &stderr); code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("missing journal: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	writeFixtureFile(t, o.keyPath, []byte("bad key"), 0o600)
	stdout.Reset()
	stderr.Reset()
	if code := Run(args, &stdout, &stderr); code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid owner key: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestSwitchJournalStatusRejectsFlagErrors(t *testing.T) {
	for _, args := range [][]string{{"switch", "status"}, {"switch", "status", "--unknown"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("invalid flags %v: code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestSwitchJournalStatusRejectsInvalidPlan(t *testing.T) {
	o, _, _ := armCommandFixture(t)
	writeFixtureFile(t, o.planPath, []byte("invalid plan"), 0o600)
	args := []string{"switch", "status", "--plan", o.planPath, "--owner-key", o.keyPath, "--journal", filepath.Join(t.TempDir(), "journal")}
	var stdout, stderr bytes.Buffer
	if code := Run(args, &stdout, &stderr); code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid plan: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestSwitchJournalStatusRejectsMalformedJournal(t *testing.T) {
	o, _, _ := armCommandFixture(t)
	journal := filepath.Join(t.TempDir(), "journal")
	if err := os.Mkdir(journal, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journal, "1.request"), []byte("malformed"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"switch", "status", "--plan", o.planPath, "--owner-key", o.keyPath, "--journal", journal}
	var stdout, stderr bytes.Buffer
	if code := Run(args, &stdout, &stderr); code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("malformed journal: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

type failingCheckinWriter struct{}

func (failingCheckinWriter) Write([]byte) (int, error) { return 0, errors.New("writer failed") }

func TestRenewalResultReportsEncodingFailures(t *testing.T) {
	var stderr bytes.Buffer
	result := switchclient.Renewal{Confirmed: true}
	if code := writeRenewalResult(failingCheckinWriter{}, &stderr, result, nil); code != 1 || stderr.Len() != 0 {
		t.Fatalf("result encoding failure: code=%d stderr=%q", code, stderr.String())
	}

	stderr.Reset()
	result = switchclient.Renewal{Request: switchwire.Envelope{Body: []byte("signed intent")}}
	if code := writeRenewalResult(failingCheckinWriter{}, &stderr, result, errors.New("renewal failed")); code != 1 || stderr.Len() != 0 {
		t.Fatalf("uncertain result encoding failure: code=%d stderr=%q", code, stderr.String())
	}
}

func TestRenewalEvidenceReportsUnavailableExportPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "renewal.json")
	err := writeRenewalEvidence(path, switchclient.Renewal{})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unavailable export path error = %v", err)
	}
}
