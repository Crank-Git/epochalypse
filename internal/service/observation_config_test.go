package service

import (
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/observation"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func configuredResolvers() []observation.Target {
	return []observation.Target{{ID: "strict", Address: "127.0.0.1:1", Version: "1.22.0", VersionSource: "configured", Clock: observation.Clock{Source: "unknown"}}}
}

func TestResolverConfigurationRequiresTheMainAuthority(t *testing.T) {
	config := Config{Environment: "lab", Mode: "server", HTTPAddr: "127.0.0.1:0", StateDir: t.TempDir(), LocalLab: true, Resolvers: configuredResolvers()}
	if err := config.Validate(); err == nil {
		t.Fatal("resolver jobs accept a service without signed windows")
	}
	config.Zone = "windows.example."
	config.KeyFile = serviceKey(t)
	config.DNSAddr = "127.0.0.1:0"
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	config.Resolvers[0].VersionSource = "executable -V"
	if err := config.Validate(); err == nil {
		t.Fatal("configuration claims a runtime version measurement")
	}
	config.Resolvers = configuredResolvers()
	config.Resolvers[0].Clock.Source = "measured"
	zero := int64(0)
	config.Resolvers[0].Clock.OffsetNanoseconds = &zero
	if err := config.Validate(); err == nil {
		t.Fatal("configuration claims a runtime clock measurement")
	}
}

func TestMainStateRecoversRunsBeforeJobAdmission(t *testing.T) {
	config := Config{StateDir: t.TempDir(), Zone: "windows.example.", KeyFile: serviceKey(t), Resolvers: configuredResolvers()}
	state, err := openMainState(config)
	if err != nil {
		t.Fatal(err)
	}
	w, err := state.windows.Create(validServiceInput(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.windows.ReserveRun(w.ID, configuredResolvers(), time.Now()); err != nil {
		t.Fatal(err)
	}
	state.close()
	state, err = openMainState(config)
	if err != nil {
		t.Fatal(err)
	}
	defer state.close()
	runs, _, err := state.windows.ListRuns(w.ID, "")
	if err != nil || len(runs) != 1 || runs[0].State != "partial" || runs[0].Results[0].Error != "run_interrupted" {
		t.Fatalf("startup leaves an interrupted job pending: %+v: %v", runs, err)
	}
	if state.jobs == nil || !state.jobs.Available() {
		t.Fatal("startup does not configure resolver jobs")
	}
	if _, err := state.jobs.Start(w.ID); err != nil {
		t.Fatal(err)
	}
	state.close()
	store := reopenServiceWindows(t, config)
	defer func() { _ = store.Close() }()
	runs, _, err = store.ListRuns(w.ID, "")
	if err != nil || len(runs) != 2 {
		t.Fatal("shutdown loses an accepted run", err)
	}
	for _, run := range runs {
		if run.State == "pending" {
			t.Fatal("shutdown closes the database before the job drains")
		}
	}
}

func reopenServiceWindows(t *testing.T, config Config) *window.Store {
	t.Helper()
	signer, err := window.LoadSigner(config.Zone, config.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	store, err := window.Open(config.StateDir, signer)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
