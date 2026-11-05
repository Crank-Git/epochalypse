package service

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPublicCapsuleSeparatesReleaseAndAvailability(t *testing.T) {
	for _, mode := range []string{"public", "server-held", "tlock"} {
		t.Run(mode, func(t *testing.T) {
			h := newCapsuleHTTP(t)
			draft := createHTTPCapsule(t, h, mode)
			path := "/c/" + draft.ID
			if w := h.send(httptest.NewRequest("GET", "https://localhost"+path, nil)); w.Code != 404 {
				t.Fatalf("draft page: %d", w.Code)
			}
			if w := h.publish(draft.ID); w.Code != 200 {
				t.Fatal(w.Body)
			}
			w := h.send(httptest.NewRequest("GET", "https://localhost"+path, nil))
			for _, text := range []string{"Selected mode", mode, "Requested release", "Effective release", "Content availability", "Public copies remain outside"} {
				if w.Code != 200 || !strings.Contains(w.Body.String(), text) {
					t.Fatalf("missing %q: %d %s", text, w.Code, w.Body)
				}
			}
			if mode == "public" {
				if !strings.Contains(w.Body.String(), "Verified and available") {
					t.Fatal(w.Body)
				}
			} else {
				if !strings.Contains(w.Body.String(), "Sealed") || strings.Contains(w.Body.String(), ">Download content</a>") {
					t.Fatal(w.Body)
				}
			}
			*h.clock = h.clock.Add(2 * time.Hour)
			w = h.send(httptest.NewRequest("GET", "https://localhost"+path, nil))
			expected := "Verified and available"
			if mode == "tlock" {
				expected = "Delayed availability"
			}
			if !strings.Contains(w.Body.String(), expected) || !strings.Contains(w.Body.String(), "Release condition is due") {
				t.Fatal(w.Body)
			}
		})
	}
}

func TestPublicCapsuleAuthenticatesAttachment(t *testing.T) {
	h := newCapsuleHTTP(t)
	draft := createHTTPCapsule(t, h, "server-held")
	h.publish(draft.ID)
	path := "https://localhost/api/v1/public/capsules/" + draft.ID + "/download"
	if w := h.send(httptest.NewRequest("GET", path, nil)); w.Code != 425 || strings.Contains(w.Body.String(), "1234") {
		t.Fatalf("early: %d %s", w.Code, w.Body)
	}
	*h.clock = draft.RequestedReleaseAt
	w := h.send(httptest.NewRequest("GET", path, nil))
	if w.Code != 200 || w.Body.String() != "1234" || w.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("download: %d %s %v", w.Code, w.Body, w.Header())
	}
}
