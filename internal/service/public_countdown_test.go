package service

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/countdown"
)

func TestPublicCountdownPreservesAbsoluteEvent(t *testing.T) {
	h, _, api := newCountdownAPIHTTP(t)
	record, err := api.store.Create(context.Background(), countdown.Input{Title: "<script>event</script>", EventAt: h.clock.Add(time.Hour).Format(time.RFC3339)}, *h.clock)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := api.store.List()
	path := "https://localhost/events/" + record.ID
	w := h.send(httptest.NewRequest("GET", path, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "&lt;script&gt;event&lt;/script&gt;") || !strings.Contains(w.Body.String(), record.Name) || !strings.Contains(w.Body.String(), "Recursive TTL values are advisory") {
		t.Fatalf("public event: %d %s", w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self'") || strings.Contains(w.Header().Get("Content-Security-Policy"), "unsafe-inline") {
		t.Fatal("the public page omits its security headers")
	}
	for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		*h.clock = record.EventAt.Add(offset)
		w = h.send(httptest.NewRequest("GET", "https://localhost/api/v1/public/countdowns/"+record.ID, nil))
		var value struct {
			countdown.Record
			countdown.Timing
		}
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil || w.Code != 200 || !value.ServerNow.Equal(*h.clock) || !value.EventAt.Equal(record.EventAt) || value.AuthoritativeTTL != 0 {
			t.Fatalf("precise public time: %d %s %v", w.Code, w.Body, err)
		}
	}
	w = h.send(httptest.NewRequest("GET", path, nil))
	if !strings.Contains(w.Body.String(), "Completed") {
		t.Fatal("the event boundary does not show completion")
	}
	after, _ := api.store.List()
	if len(after) != len(before) || after[0] != before[0] {
		t.Fatal("public reads change retained state")
	}
}

func TestPublicCountdownRejectsInvalidReads(t *testing.T) {
	h, _, api := newCountdownAPIHTTP(t)
	for _, row := range []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/events/invalid", "", 400},
		{"GET", "/events/" + strings.Repeat("a", 32), "", 404},
		{"GET", "/events/" + strings.Repeat("a", 32) + "?query=1", "", 400},
		{"GET", "/events/" + strings.Repeat("a", 32), "body", 400},
		{"DELETE", "/events/" + strings.Repeat("a", 32), "", 405},
		{"HEAD", "/events/" + strings.Repeat("a", 32), "", 405},
		{"POST", "/api/v1/public/countdowns/" + strings.Repeat("a", 32), "", 405},
	} {
		w := h.send(httptest.NewRequest(row.method, "https://localhost"+row.path, strings.NewReader(row.body)))
		if w.Code != row.code {
			t.Errorf("%s %s: %d %s", row.method, row.path, w.Code, w.Body)
		}
	}
	api.slots <- struct{}{}
	if w := h.send(httptest.NewRequest("GET", "https://localhost/events/invalid", nil)); w.Code != 429 {
		t.Fatalf("unbounded work: %d", w.Code)
	}
	<-api.slots
	_ = api.store.Close()
	if w := h.send(httptest.NewRequest("GET", "https://localhost/events/"+strings.Repeat("a", 32), nil)); w.Code != 503 {
		t.Fatalf("storage error: %d", w.Code)
	}
}

func TestPublicCountdownRequiresTLSAndServesLocalAssets(t *testing.T) {
	h, _, api := newCountdownAPIHTTP(t)
	for _, path := range []string{"/events/invalid", "/api/v1/public/countdowns/invalid", "/events/countdown.js", "/events/style.css"} {
		if w := h.send(httptest.NewRequest("GET", "http://localhost"+path, nil)); w.Code != 426 {
			t.Fatalf("public plaintext read: %d %s", w.Code, w.Body)
		}
	}
	for _, path := range []string{"/events/countdown.js", "/events/style.css"} {
		if w := h.send(httptest.NewRequest("GET", "https://localhost"+path, nil)); w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("public asset: %d", w.Code)
		}
	}
	api.slots = nil
	if w := h.send(httptest.NewRequest("GET", "https://localhost/events/invalid", nil)); w.Code != 503 {
		t.Fatalf("unavailable admission: %d", w.Code)
	}
}
