package service

import (
	"encoding/json"
	"github.com/Crank-Git/epochalypse/internal/window"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/observation"
	"github.com/Crank-Git/epochalypse/internal/resolver"
)

func observationHTTP(t *testing.T) (windowHTTP, string) {
	t.Helper()
	h := newWindowHTTP(t)
	targets := []observation.Target{{ID: "strict", Address: "127.0.0.1:1", Version: "unknown", VersionSource: "unknown", Clock: observation.Clock{Source: "unknown"}}}
	jobs, err := resolver.NewJobs(h.state.windows, targets)
	if err != nil {
		t.Fatal(err)
	}
	h.state.jobs = jobs
	t.Cleanup(jobs.Close)
	w, err := h.state.windows.Create(validServiceInput(), h.now)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.state.routes(mux, false)
	h.handler = mux
	return h, "/api/v1/windows/" + w.ID + "/observations"
}

func validServiceInput() window.Input {
	return window.Input{Content: "public observation", Inception: "2026-11-04T13:00:00Z", Expiration: "2026-11-04T14:00:00Z"}
}

func observationRequest(h windowHTTP, path, body string) *http.Request {
	r := h.request(body)
	r.URL.Path = path
	return r
}

func TestObservationHTTPStartsDurableJobsAndAllowsPublicReads(t *testing.T) {
	h, path := observationHTTP(t)
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, observationRequest(h, path, "{}"))
	if w.Code != 202 {
		t.Fatalf("the run does not start: %d %s", w.Code, w.Body)
	}
	var run observation.Run
	if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if run.State != "pending" || run.ID == "" || strings.Contains(w.Body.String(), "127.0.0.1:1") {
		t.Fatal("admission loses its public pending result")
	}
	h.state.jobs.Close()
	h.state.jobs = nil
	read := httptest.NewRecorder()
	h.handler.ServeHTTP(read, httptest.NewRequest("GET", "https://localhost"+path, nil))
	if read.Code != 200 || !strings.Contains(read.Body.String(), "resolver_unavailable") || strings.Contains(read.Body.String(), "127.0.0.1:1") {
		t.Fatalf("the public result loses the unavailable resolver: %d %s", read.Code, read.Body)
	}
	if read.Header().Get("Cache-Control") != "no-store" || read.Header().Get("X-Request-ID") == "" {
		t.Fatal("public reads omit shared safeguards")
	}
}

func TestObservationHTTPRejectsMutationPolicyFailures(t *testing.T) {
	h, path := observationHTTP(t)
	for name, change := range map[string]struct {
		status int
		apply  func(*http.Request)
	}{
		"owner session":   {401, func(r *http.Request) { r.Header.Del("Cookie") }},
		"bearer fallback": {401, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }},
		"csrf":            {403, func(r *http.Request) { r.Header.Del("X-CSRF-Token") }},
		"origin":          {403, func(r *http.Request) { r.Header.Set("Origin", "https://other.invalid") }},
		"tls":             {426, func(r *http.Request) { r.TLS = nil; r.URL.Scheme = "http" }},
	} {
		t.Run(name, func(t *testing.T) {
			r := observationRequest(h, path, "{}")
			change.apply(r)
			w := httptest.NewRecorder()
			h.handler.ServeHTTP(w, r)
			if w.Code != change.status {
				t.Fatalf("policy returns %d, want %d", w.Code, change.status)
			}
		})
	}
}
