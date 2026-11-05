package service

import (
	"context"
	"encoding/json"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newSwitchHTTP(t *testing.T) (capsuleHTTPFixture, string) {
	t.Helper()
	h := newCapsuleHTTP(t)
	grant, err := h.state.owner.CreateToken(owner.TokenRequest{Name: "Synthetic switch writer", Scopes: []string{"switch:write"}, ExpiresAt: time.Now().Add(time.Hour)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.state.routes(mux, false)
	h.handler = mux
	return h, grant.Token
}

func TestSwitchHTTPRetainsUnarmedConfiguration(t *testing.T) {
	h, token := newSwitchHTTP(t)
	w := h.send(countdownRequest("POST", "/api/v1/switches", `{}`, token))
	if w.Code != 201 {
		t.Fatalf("draft creation: %d %s", w.Code, w.Body)
	}
	var record struct {
		ID       string `json:"id"`
		State    string `json:"state"`
		Interval int64  `json:"interval_seconds"`
		Grace    int64  `json:"grace_seconds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.State != "draft" || record.Interval != 86400 || record.Grace != 3600 || strings.Contains(w.Body.String(), "deadline") {
		t.Fatalf("draft claims authority: %s", w.Body)
	}
	w = h.send(countdownRequest("GET", "/api/v1/switches/"+record.ID, "", token))
	if w.Code != 200 || !strings.Contains(w.Body.String(), record.ID) {
		t.Fatalf("draft inspection: %d %s", w.Code, w.Body)
	}
}

func TestSwitchHTTPArmRejectsInvalidEvidenceAndRequiresOwnerProof(t *testing.T) {
	h, token := newSwitchHTTP(t)
	for _, credential := range []string{"", h.bearer, token} {
		w := h.send(countdownRequest("POST", "/api/v1/switches/"+strings.Repeat("a", 32)+"/arm", `{}`, credential))
		if credential == token {
			if w.Code != 400 {
				t.Fatalf("invalid arm evidence: %d %s", w.Code, w.Body)
			}
		} else if w.Code != 401 && w.Code != 403 {
			t.Fatalf("unauthorized arm: %d", w.Code)
		}
	}
}

func TestSwitchHTTPRejectsInvalidRequests(t *testing.T) {
	h, token := newSwitchHTTP(t)
	for _, body := range []string{"", "null", "[]", `{} {}`, `{"unknown":1}`, `{"interval_seconds":3599}`, `{"interval_seconds":31536001}`, `{"grace_seconds":-1}`, `{"grace_seconds":2592001}`, `{"interval_seconds":1.5}`, string([]byte{255})} {
		w := h.send(countdownRequest("POST", "/api/v1/switches", body, token))
		if w.Code != 400 {
			t.Fatalf("body %q: %d %s", body, w.Code, w.Body)
		}
	}
	for _, step := range []struct {
		method, path, body string
		status             int
	}{
		{"PUT", "/api/v1/switches", "", 405}, {"POST", "/api/v1/switches/id", "{}", 405},
		{"POST", "/api/v1/switches?x=1", "{}", 400}, {"GET", "/api/v1/switches/invalid", "", 400},
		{"GET", "/api/v1/switches/" + strings.Repeat("a", 32), "", 404},
		{"GET", "/api/v1/switches/" + strings.Repeat("a", 32), "x", 400},
		{"POST", "/api/v1/switches", strings.Repeat("x", 4097), 413},
	} {
		w := h.send(countdownRequest(step.method, step.path, step.body, token))
		if w.Code != step.status {
			t.Fatalf("%s %s: %d %s", step.method, step.path, w.Code, w.Body)
		}
	}
	r := countdownRequest("POST", "/api/v1/switches", "{}", token)
	r.Header.Set("Content-Type", "text/plain")
	if w := h.send(r); w.Code != 400 {
		t.Fatalf("content type: %d", w.Code)
	}
}

func TestSwitchHTTPRequiresOwnerProof(t *testing.T) {
	h, token := newSwitchHTTP(t)
	for _, credential := range []string{"", h.bearer} {
		w := h.send(countdownRequest("POST", "/api/v1/switches", "{}", credential))
		if w.Code != 401 && w.Code != 403 {
			t.Fatalf("unauthorized draft: %d", w.Code)
		}
	}
	r := countdownRequest("POST", "/api/v1/switches", "{}", token)
	r.URL.Scheme = "http"
	r.TLS = nil
	if w := h.send(r); w.Code != 426 {
		t.Fatalf("plain HTTP: %d", w.Code)
	}
	r = countdownRequest("POST", "/api/v1/switches", "{}", "")
	r.Header.Del("Authorization")
	r.AddCookie(&http.Cookie{Name: "__Host-epochalypse-session", Value: h.session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	if w := h.send(r); w.Code != 403 {
		t.Fatalf("absent CSRF: %d", w.Code)
	}
	r.Header.Set("X-CSRF-Token", h.proof)
	r.AddCookie(h.proofCookie)
	if w := h.send(r); w.Code != 201 {
		t.Fatalf("owner proof: %d %s", w.Code, w.Body)
	}
}

func TestSwitchHTTPBoundsWork(t *testing.T) {
	h, token := newSwitchHTTP(t)
	store, err := switches.Attach(h.state.content)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	api := &switchAPI{store: store, now: time.Now, slots: make(chan struct{}, 1)}
	mux := http.NewServeMux()
	api.routes(mux, h.state.owner, false)
	h.handler = mux
	api.slots <- struct{}{}
	if w := h.send(countdownRequest("POST", "/api/v1/switches", "{}", token)); w.Code != 429 || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("busy work: %d %s", w.Code, w.Body)
	}
	<-api.slots
	r := countdownRequest("POST", "/api/v1/switches", "{}", token)
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	if w := h.send(r.WithContext(ctx)); w.Code != 408 {
		t.Fatalf("canceled work: %d", w.Code)
	}
	api.store = nil
	if w := h.send(r); w.Code != 503 {
		t.Fatalf("unavailable storage: %d", w.Code)
	}
}

func TestSwitchOwnerPageRequiresSession(t *testing.T) {
	h, token := newSwitchHTTP(t)
	r := httptest.NewRequest("GET", "https://localhost/owner/switches", nil)
	if w := h.send(r); w.Code != 401 {
		t.Fatalf("unauthorized page: %d", w.Code)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	if w := h.send(r); w.Code != 401 {
		t.Fatalf("scoped token opens owner page: %d", w.Code)
	}
	r.Header.Del("Authorization")
	r.AddCookie(&http.Cookie{Name: "__Host-epochalypse-session", Value: h.session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w := h.send(r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "irreversible release") || !strings.Contains(w.Body.String(), "An absent receipt does not prove") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatalf("owner page: %d %s", w.Code, w.Body)
	}
	r.URL.Path = "/owner/switches.js"
	if w := h.send(r); w.Code != 200 || !strings.Contains(w.Body.String(), "Deadline local:") {
		t.Fatalf("owner script: %d %s", w.Code, w.Body)
	}
	_ = h.state.content.Close()
	r.URL.Path = "/owner/switches"
	if w := h.send(r); w.Code != 503 {
		t.Fatalf("closed storage: %d", w.Code)
	}
}

func TestSwitchHTTPReportsQuotaAndClosedStorage(t *testing.T) {
	h, token := newSwitchHTTP(t)
	h.state.content.Limits.MaxObjects = 1
	if w := h.send(countdownRequest("POST", "/api/v1/switches", "{}", token)); w.Code != 201 {
		t.Fatalf("first draft: %d", w.Code)
	}
	if w := h.send(countdownRequest("POST", "/api/v1/switches", "{}", token)); w.Code != 409 {
		t.Fatalf("full quota: %d %s", w.Code, w.Body)
	}
	_ = h.state.content.Close()
	if w := h.send(countdownRequest("POST", "/api/v1/switches", "{}", token)); w.Code != 503 {
		t.Fatalf("closed storage: %d", w.Code)
	}
}
