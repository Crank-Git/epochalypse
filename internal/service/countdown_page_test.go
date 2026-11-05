package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCountdownOwnerPageRequiresSession(t *testing.T) {
	h, _, _ := newCountdownAPIHTTP(t)
	r := httptest.NewRequest("GET", "https://localhost/owner/countdowns", nil)
	if w := h.send(r); w.Code != 401 {
		t.Fatalf("the owner page bypasses authorization: %d", w.Code)
	}
	r.AddCookie(&http.Cookie{Name: "__Host-epochalypse-session", Value: h.session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	if w := h.send(r); w.Code != 200 || !strings.Contains(w.Body.String(), "Create a countdown") {
		t.Fatalf("the owner controls are unavailable: %d %s", w.Code, w.Body)
	}
}
