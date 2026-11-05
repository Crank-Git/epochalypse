package service

import (
	"encoding/json"
	"github.com/Crank-Git/epochalypse/internal/window"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/observation"
	"github.com/Crank-Git/epochalypse/internal/resolver"
)

func observationHTTP(t *testing.T, limits ...window.Limits) (windowHTTP, string) {
	t.Helper()
	h := newWindowHTTP(t, limits...)
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

func TestObservationHTTPRejectsRequestSuppliedTargetsAndMalformedInputs(t *testing.T) {
	h, path := observationHTTP(t)
	cases := []struct {
		method, body, query, media string
		status                     int
	}{
		{"POST", `{"address":"http://169.254.169.254/"}`, "", "application/json", 400},
		{"POST", "{} {}", "", "application/json", 400},
		{"POST", "null", "", "application/json", 400},
		{"POST", "{", "", "application/json", 400},
		{"POST", "{}", "", "text/plain", 400},
		{"POST", "{}", "resolver=127.0.0.1:1", "application/json", 400},
		{"POST", strings.Repeat(" ", 1025), "", "application/json", 413},
		{"GET", "", "after=invalid", "", 400},
		{"GET", "", "after=&after=", "", 400},
		{"GET", "", "resolver=arbitrary", "", 400},
		{"GET", "", "%zz", "", 400},
		{"DELETE", "", "", "", 405},
	}
	for _, c := range cases {
		r := observationRequest(h, path, c.body)
		r.Method = c.method
		r.URL.RawQuery = c.query
		r.Header.Set("Content-Type", c.media)
		w := httptest.NewRecorder()
		h.handler.ServeHTTP(w, r)
		if w.Code != c.status || !strings.Contains(w.Body.String(), `"request_id"`) {
			t.Fatalf("%s %q %q returns %d: %s", c.method, c.body, c.query, w.Code, w.Body)
		}
	}
	bad := observationRequest(h, path, strings.Repeat(" ", 1025))
	bad.ContentLength = -1
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, bad)
	if w.Code != 413 {
		t.Fatal("a chunked body bypasses the request bound")
	}
	runs, _, err := h.state.windows.ListRuns(strings.Split(path, "/")[4], "")
	if err != nil || len(runs) != 0 {
		t.Fatal("a rejected request starts network work", err)
	}
}

func TestObservationHTTPCoversUnavailableQuotaAndMissingWindows(t *testing.T) {
	h, path := observationHTTP(t, window.Limits{MaxObjects: 1, MaxBytes: 1 << 30})
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, observationRequest(h, path, ""))
	if w.Code != 409 {
		t.Fatalf("the shared quota returns %d: %s", w.Code, w.Body)
	}
	missing := strings.Replace(path, strings.Split(path, "/")[4], strings.Repeat("0", 32), 1)
	w = httptest.NewRecorder()
	h.handler.ServeHTTP(w, observationRequest(h, missing, "{}"))
	if w.Code != 404 {
		t.Fatal("a missing window does not return 404")
	}
	h.state.jobs.Close()
	for _, method := range []string{"GET", "POST"} {
		r := observationRequest(h, path, "{}")
		r.Method = method
		w = httptest.NewRecorder()
		h.handler.ServeHTTP(w, r)
		if w.Code != 503 {
			t.Fatalf("the closed service returns %d", w.Code)
		}
	}
	h.state.jobs = nil
	w = httptest.NewRecorder()
	h.handler.ServeHTTP(w, observationRequest(h, path, "{}"))
	if w.Code != 503 {
		t.Fatal("an unconfigured runner accepts a job")
	}
	for _, method := range []string{"GET", "POST"} {
		r := observationRequest(h, "/api/v1/windows/invalid/observations", "{}")
		r.Method = method
		w = httptest.NewRecorder()
		h.handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal("an invalid window identifier passes")
		}
	}
}

func TestObservationHTTPPublishesBoundedPages(t *testing.T) {
	h, path := observationHTTP(t)
	id := strings.Split(path, "/")[4]
	for range 51 {
		if _, err := h.state.windows.ReserveRun(id, configuredResolvers(), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	read := func(query string) struct {
		Runs []observation.Run `json:"runs"`
		Next string            `json:"next_cursor"`
	} { t.Helper(); w := httptest.NewRecorder(); h.handler.ServeHTTP(w, httptest.NewRequest("GET", "https://localhost"+path+query, nil)); if w.Code != 200 {
		t.Fatalf("public page returns %d", w.Code)
	}; var page struct {
		Runs []observation.Run `json:"runs"`
		Next string            `json:"next_cursor"`
	}; if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}; return page }
	first := read("")
	if len(first.Runs) != 50 || first.Next == "" {
		t.Fatal("the first page loses its bound or cursor")
	}
	last := read("?after=" + first.Next)
	if len(last.Runs) != 1 || last.Next != "" {
		t.Fatal("the final page repeats or loses a run")
	}
}

func TestObservationHTTPBoundsActiveJobsAndCancelsBeforeStoreClose(t *testing.T) {
	h, path := observationHTTP(t)
	connection, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	h.state.jobs.Close()
	targets := configuredResolvers()
	targets[0].Address = connection.LocalAddr().String()
	h.state.jobs, err = resolver.NewJobs(h.state.windows, targets)
	if err != nil {
		t.Fatal(err)
	}
	defer h.state.jobs.Close()
	for range 4 {
		w := httptest.NewRecorder()
		h.handler.ServeHTTP(w, observationRequest(h, path, "{}"))
		if w.Code != 202 {
			t.Fatalf("admission returns %d", w.Code)
		}
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, _, err := connection.ReadFrom(make([]byte, 1232)); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, observationRequest(h, path, "{}"))
	if w.Code != 429 || w.Header().Get("Retry-After") != "1" {
		t.Fatal("HTTP admission bypasses the active work bound")
	}
	before := time.Now()
	h.state.jobs.Close()
	if time.Since(before) > time.Second {
		t.Fatal("shutdown waits for a full query timeout instead of cancellation")
	}
}
