package agent

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadinessReportsSyntheticRuntimeWithoutAuthorityMutation(t *testing.T) {
	service, config := readinessServiceFixture(t)
	before, _ := service.journal.Snapshot()
	for _, path := range []string{"/readyz", "/version", "/healthz"} {
		response := httptest.NewRecorder()
		service.Handler().ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 200 {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body)
		}
		if path == "/readyz" {
			var report struct {
				Ready      bool   `json:"ready"`
				Production bool   `json:"production_ready"`
				Agent      string `json:"agent_id"`
			}
			if json.Unmarshal(response.Body.Bytes(), &report) != nil || !report.Ready || report.Production || report.Agent != config.Transition.AgentID {
				t.Fatalf("readiness: %s", response.Body)
			}
		}
	}
	after, _ := service.journal.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("metadata changes durable authority")
	}
	service.transitions.clock = func() (Reading, error) { return Reading{}, ErrClock }
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
	if response.Code != 503 {
		t.Fatalf("unsafe clock: %d %s", response.Code, response.Body)
	}
	after, _ = service.journal.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("readiness persists a clock decision")
	}
}

func TestReadinessRejectsMissingStorageWithoutSecretOutput(t *testing.T) {
	service, config := readinessServiceFixture(t)
	if err := os.Remove(filepath.Join(service.journal.config.CheckpointDirectory, checkpointSpareName)); err != nil {
		t.Fatal(err)
	}
	if report := service.Readiness(); report.Ready || report.StateReady {
		t.Fatalf("missing storage: %+v", report)
	}
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/version", nil))
	for _, secret := range []string{config.Transition.AgeIdentity, string(config.Transition.SigningKey), config.Authority.ExpectedVolumeID, config.Authority.Directory} {
		if secret != "" && strings.Contains(response.Body.String(), secret) {
			t.Fatal("metadata exposes private configuration")
		}
	}
}

func readinessServiceFixture(t *testing.T) (*Service, ServiceConfig) {
	t.Helper()
	f := newTransitionFixture(t)
	if err := f.journal.Close(); err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{Authority: f.authorityConfig, Transition: f.config(0), LabOnlySyntheticClock: true}
	service, err := OpenService(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service, config
}

func TestReadinessRejectsCheckpointSlotsWithOneInode(t *testing.T) {
	service, _ := readinessServiceFixture(t)
	root := service.journal.config.CheckpointDirectory
	if err := os.Remove(filepath.Join(root, checkpointSpareName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, authorityCheckpointName), filepath.Join(root, checkpointSpareName)); err != nil {
		t.Fatal(err)
	}
	if service.Readiness().Ready {
		t.Fatal("shared checkpoint inode passes readiness")
	}
}
