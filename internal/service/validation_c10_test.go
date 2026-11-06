package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/observation"
	"github.com/Crank-Git/epochalypse/internal/owner"
	bolt "go.etcd.io/bbolt"
)

func TestCapsuleDeleteRejectsNoncanonicalConfirmationWithoutMutation(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "server-held")
	for _, tc := range []struct {
		name, contentType, body string
	}{
		{"missing type", "", `{"confirm":"` + manifest.ID + `"}`},
		{"oversized", "application/json", strings.Repeat("x", 129)},
		{"nonstring", "application/json", `{"confirm":true}`},
		{"duplicate", "application/json", `{"confirm":"` + manifest.ID + `","confirm":"` + manifest.ID + `"}`},
		{"whitespace", "application/json", ` {"confirm":"` + manifest.ID + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("DELETE", "https://localhost/api/v1/capsules/"+manifest.ID, strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+h.bearer)
			if tc.contentType != "" {
				r.Header.Set("Content-Type", tc.contentType)
			}
			w := h.send(r)
			if w.Code != 400 && w.Code != 413 {
				t.Fatalf("invalid confirmation: %d %s", w.Code, w.Body)
			}
			if _, err := h.store.Get(manifest.ID); err != nil {
				t.Fatalf("invalid confirmation removes retained state: %v", err)
			}
		})
	}
}

func TestMainStartupRejectsCorruptObservationAndReleasesOwner(t *testing.T) {
	config := Config{StateDir: t.TempDir(), Environment: "lab", Zone: "windows.example.", KeyFile: serviceKey(t)}
	state, err := openMainState(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.content.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(installation.ObservationsBucket)).Put([]byte("corrupt-run"), []byte("invalid run"))
	}); err != nil {
		state.close()
		t.Fatal(err)
	}
	state.close()
	state, err = openMainState(config)
	if state != nil {
		state.close()
	}
	if err == nil {
		t.Fatal("startup serves corrupt retained observations")
	}
	owners, err := owner.Open(config.StateDir)
	if err != nil {
		t.Fatalf("failed observation recovery retains owner lock: %v", err)
	}
	if err := owners.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProductionCountdownStartupRejectsLabSigningKey(t *testing.T) {
	key := countdownServiceKey(t)
	data, err := os.ReadFile(key) // #nosec G304 -- The fixture creates this synthetic key.
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("Comment: epochalypse-lab-only\n")...)
	if err := os.WriteFile(key, data, 0600); err != nil { // #nosec G703 -- The fixture creates this synthetic key.
		t.Fatal(err)
	}
	config := Config{StateDir: t.TempDir(), CountdownZone: "countdown.example.", CountdownKeyFile: key}
	state, err := openMainState(config)
	if state != nil {
		state.close()
	}
	if err == nil || !strings.Contains(err.Error(), "reject an explicit lab key") {
		t.Fatalf("production countdown accepts a lab signing key: %v", err)
	}
	config.Environment = "lab"
	state, err = openMainState(config)
	if err != nil {
		t.Fatalf("rejected lab key retains a store lock: %v", err)
	}
	state.close()
}

type rejectedDeadlineC10 struct {
	*httptest.ResponseRecorder
	cause error
}

func (w rejectedDeadlineC10) SetReadDeadline(time.Time) error { return w.cause }

func TestSwitchHTTPWithholdsMutationWhenReadDeadlineFails(t *testing.T) {
	for _, arm := range []bool{false, true} {
		h, token := newSwitchHTTP(t)
		api := &switchAPI{store: h.state.switches, slots: make(chan struct{}, 1), now: time.Now}
		mux := http.NewServeMux()
		api.routes(mux, h.state.owner, false)
		path := "/api/v1/switches"
		if arm {
			path += "/" + strings.Repeat("a", 32) + "/arm"
		}
		r := countdownRequest("POST", path, `{}`, token)
		w := rejectedDeadlineC10{ResponseRecorder: httptest.NewRecorder(), cause: errors.New("synthetic deadline failure")}
		mux.ServeHTTP(w, r)
		if w.Code != 503 || strings.Contains(w.Body.String(), "synthetic deadline failure") {
			t.Fatalf("deadline failure response: %d %s", w.Code, w.Body)
		}
		records, err := h.state.switches.List()
		if err != nil || len(records) != 0 {
			t.Fatalf("deadline failure changes retained state: %v %v", records, err)
		}
	}
}

type rejectedSwitchBodyC10 struct{ cause error }

func (r rejectedSwitchBodyC10) Read([]byte) (int, error) { return 0, r.cause }
func (rejectedSwitchBodyC10) Close() error               { return nil }

func TestSwitchArmBodyPreservesReadFailure(t *testing.T) {
	cause := errors.New("synthetic arm body failure")
	r := httptest.NewRequest("POST", "https://localhost/api/v1/switches/id/arm", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Body = rejectedSwitchBodyC10{cause: cause}
	if _, err := switchArmBody(httptest.NewRecorder(), r); !errors.Is(err, cause) {
		t.Fatalf("arm body loses read failure: %v", err)
	}
}

func TestCountdownHTTPWithholdsMutationWhenReadDeadlineFails(t *testing.T) {
	h, token, api := newCountdownAPIHTTP(t)
	r := countdownRequest("POST", "/api/v1/countdowns", countdownJSONInput, token)
	w := rejectedDeadlineC10{ResponseRecorder: httptest.NewRecorder(), cause: errors.New("synthetic countdown deadline failure")}
	h.handler.ServeHTTP(w, r)
	if w.Code != 503 || strings.Contains(w.Body.String(), "synthetic countdown deadline failure") {
		t.Fatalf("countdown deadline failure: %d %s", w.Code, w.Body)
	}
	records, err := api.store.List()
	if err != nil || len(records) != 0 {
		t.Fatalf("countdown deadline failure changes retained state: %v %v", records, err)
	}
}

func TestResolverConfigRejectsInvalidTargetBeforeStartup(t *testing.T) {
	config := Config{Mode: "server", Zone: "windows.example.", Resolvers: []observation.Target{{ID: "invalid"}}}
	if err := config.validateResolvers(); err == nil {
		t.Fatal("resolver configuration accepts an incomplete target")
	}
}

func TestCapsuleDeleteCancellationPreservesRetainedState(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "server-held")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest("DELETE", "https://localhost/api/v1/capsules/"+manifest.ID, nil).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+h.bearer)
	r.Body = cancelBodyC10{reader: strings.NewReader(`{"confirm":"` + manifest.ID + `"}`), cancel: cancel}
	w := h.send(r)
	if w.Code != 408 {
		t.Fatalf("cancelled deletion: %d %s", w.Code, w.Body)
	}
	if _, err := h.store.Get(manifest.ID); err != nil {
		t.Fatalf("cancelled deletion removes retained state: %v", err)
	}
}

type cancelBodyC10 struct {
	reader *strings.Reader
	cancel context.CancelFunc
}

func (b cancelBodyC10) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if b.reader.Len() == 0 {
		b.cancel()
	}
	return n, err
}

func (cancelBodyC10) Close() error { return nil }

func TestResolverConfigRejectsUnknownProvenanceWithClaimedEvidence(t *testing.T) {
	for _, target := range []observation.Target{
		{ID: "strict", Address: "127.0.0.1:1", Version: "1.22.0", VersionSource: "unknown", Clock: observation.Clock{Source: "unknown"}},
		{ID: "strict", Address: "127.0.0.1:1", Version: "unknown", VersionSource: "unknown", Clock: observation.Clock{Source: "unknown", LowerNanoseconds: new(int64), UpperNanoseconds: new(int64)}},
	} {
		config := Config{Mode: "server", Zone: "windows.example.", Resolvers: []observation.Target{target}}
		if err := config.validateResolvers(); err == nil {
			t.Fatal("unknown provenance accepts claimed evidence")
		}
	}
}

func TestMainStateReleasesStoresAfterAuthorityOrResolverFailure(t *testing.T) {
	for _, failure := range []string{"authority", "resolver"} {
		t.Run(failure, func(t *testing.T) {
			config := Config{StateDir: t.TempDir(), Environment: "lab", Zone: "windows.example.", KeyFile: serviceKey(t)}
			if failure == "authority" {
				config.NameServer = "ns.windows.example."
			} else {
				config.Resolvers = []observation.Target{{ID: "invalid"}}
			}
			state, err := openMainState(config)
			if state != nil {
				state.close()
			}
			if err == nil {
				t.Fatal("invalid startup dependency accepts traffic")
			}
			config.NameServer, config.Resolvers = "", nil
			state, err = openMainState(config)
			if err != nil {
				t.Fatalf("failed startup retains a store lock: %v", err)
			}
			state.close()
		})
	}
}
