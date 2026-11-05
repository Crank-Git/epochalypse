package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/installation"
)

const countdownJSONInput = `{"title":"Synthetic event","event_at":"2026-11-05T13:00:00Z"}`

func newCountdownAPIHTTP(t *testing.T) (capsuleHTTPFixture, string, *countdownAPI) {
	t.Helper()
	h, token := newCountdownHTTP(t)
	store, err := countdown.Attach(h.state.content, "countdown.example.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	api := &countdownAPI{store: store, now: func() time.Time { return *h.clock }, slots: make(chan struct{}, 1)}
	mux := http.NewServeMux()
	api.routes(mux, h.state.owner, false)
	h.handler = mux
	return h, token, api
}

func TestCountdownHTTPReleasesQuotaOnDeletion(t *testing.T) {
	h, token, api := newCountdownAPIHTTP(t)
	store, err := countdown.Open(t.TempDir(), installation.Limits{MaxObjects: 1, MaxBytes: 8192}, "countdown.example.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	api.store = store
	w := h.send(countdownRequest("POST", "/api/v1/countdowns", countdownJSONInput, token))
	if w.Code != 201 {
		t.Fatalf("creation fails: %d %s", w.Code, w.Body)
	}
	var record countdown.Record
	if err := json.Unmarshal(w.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if w := h.send(countdownRequest("POST", "/api/v1/countdowns", countdownJSONInput, token)); w.Code != 409 {
		t.Fatalf("the shared quota passes: %d %s", w.Code, w.Body)
	}
	if w := h.send(countdownRequest("DELETE", "/api/v1/countdowns/"+record.ID, "", token)); w.Code != 204 {
		t.Fatalf("deletion fails: %d %s", w.Code, w.Body)
	}
	if w := h.send(countdownRequest("POST", "/api/v1/countdowns", countdownJSONInput, token)); w.Code != 201 {
		t.Fatalf("deletion retains quota: %d %s", w.Code, w.Body)
	}
}

func TestCountdownHTTPRejectsUnauthorizedChanges(t *testing.T) {
	h, token, _ := newCountdownAPIHTTP(t)
	for _, method := range []string{"POST", "GET", "DELETE"} {
		path := "/api/v1/countdowns"
		if method != "POST" {
			path += "/" + strings.Repeat("a", 32)
		}
		for _, credential := range []string{"", h.bearer} {
			w := h.send(countdownRequest(method, path, countdownJSONInput, credential))
			if w.Code != 401 && w.Code != 403 {
				t.Fatalf("%s accepts an unauthorized credential: %d %s", method, w.Code, w.Body)
			}
		}
	}
	r := countdownRequest("POST", "/api/v1/countdowns", countdownJSONInput, token)
	r.URL.Scheme = "http"
	r.TLS = nil
	if w := h.send(r); w.Code != 426 {
		t.Fatalf("plain HTTP passes: %d %s", w.Code, w.Body)
	}
	r = countdownRequest("POST", "/api/v1/countdowns", countdownJSONInput, "")
	r.Header.Del("Authorization")
	r.AddCookie(&http.Cookie{Name: "__Host-epochalypse-session", Value: h.session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	if w := h.send(r); w.Code != 403 {
		t.Fatalf("a session without CSRF passes: %d %s", w.Code, w.Body)
	}
	r.Header.Set("X-CSRF-Token", h.proof)
	r.AddCookie(h.proofCookie)
	if w := h.send(r); w.Code != 201 {
		t.Fatalf("the owner with CSRF fails: %d %s", w.Code, w.Body)
	}
}

func TestCountdownHTTPRejectsInvalidBodiesAndTimes(t *testing.T) {
	h, token, api := newCountdownAPIHTTP(t)
	for _, body := range []string{"", "null", "{}", "[]", countdownJSONInput + "{}", `{"title":"event","event_at":false}`, `{"title":"event","event_at":"2026-11-05T12:00:00Z"}`, `{"title":"event","event_at":"2100-01-01T00:00:00Z"}`, `{"title":"event","event_at":"2026-11-05T13:00:00.5Z"}`, `{"title":"event","event_at":"2026-11-05T13:00:00Z","unknown":true}`, string([]byte{255})} {
		if w := h.send(countdownRequest("POST", "/api/v1/countdowns", body, token)); w.Code != 400 {
			t.Fatalf("invalid body %q: %d %s", body, w.Code, w.Body)
		}
	}
	if w := h.send(countdownRequest("POST", "/api/v1/countdowns", strings.Repeat("x", 4097), token)); w.Code != 413 {
		t.Fatalf("oversize input: %d", w.Code)
	}
	r := countdownRequest("POST", "/api/v1/countdowns", countdownJSONInput, token)
	r.Header.Set("Content-Type", "text/plain")
	if w := h.send(r); w.Code != 400 {
		t.Fatalf("a wrong content type passes: %d", w.Code)
	}
	list, err := api.store.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("invalid input allocates retained state: %+v %v", list, err)
	}
}

func TestCountdownHTTPBoundsWorkAndMethods(t *testing.T) {
	h, token, api := newCountdownAPIHTTP(t)
	for _, request := range []struct {
		method, path, body string
		status             int
	}{
		{"PUT", "/api/v1/countdowns", "", 405},
		{"POST", "/api/v1/countdowns/" + strings.Repeat("a", 32), "", 405},
		{"POST", "/api/v1/countdowns?title=x", countdownJSONInput, 400},
		{"GET", "/api/v1/countdowns/invalid", "", 400},
		{"DELETE", "/api/v1/countdowns/invalid", "", 400},
		{"GET", "/api/v1/countdowns/" + strings.Repeat("a", 32), "x", 400},
		{"DELETE", "/api/v1/countdowns/" + strings.Repeat("a", 32), "xx", 400},
		{"DELETE", "/api/v1/countdowns/" + strings.Repeat("a", 32), "", 404},
	} {
		if w := h.send(countdownRequest(request.method, request.path, request.body, token)); w.Code != request.status {
			t.Fatalf("%s %s: %d %s", request.method, request.path, w.Code, w.Body)
		}
	}
	api.slots <- struct{}{}
	if w := h.send(countdownRequest("POST", "/api/v1/countdowns", countdownJSONInput, token)); w.Code != 429 || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("busy work passes: %d %s", w.Code, w.Body)
	}
	<-api.slots
	r := countdownRequest("POST", "/api/v1/countdowns", countdownJSONInput, token)
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	if w := h.send(r.WithContext(ctx)); w.Code != 408 {
		t.Fatalf("canceled work passes: %d %s", w.Code, w.Body)
	}
	_ = api.store.Close()
	if w := h.send(countdownRequest("POST", "/api/v1/countdowns", countdownJSONInput, token)); w.Code != 503 {
		t.Fatalf("closed storage passes: %d %s", w.Code, w.Body)
	}
}
