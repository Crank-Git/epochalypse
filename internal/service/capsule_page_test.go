package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCapsuleControlsRequireOwnerAndDiscloseTrustBeforeUpload(t *testing.T) {
	h := newCapsuleHTTP(t)
	for _, path := range []string{"/owner/capsules", "/owner/capsules.js"} {
		request := httptest.NewRequest("GET", "https://localhost"+path, nil)
		denied := httptest.NewRecorder()
		h.handler.ServeHTTP(denied, request)
		if denied.Code != 401 {
			t.Fatalf("anonymous %s: %d", path, denied.Code)
		}
		request.AddCookie(&http.Cookie{Name: "__Host-epochalypse-session", Value: h.session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		response := httptest.NewRecorder()
		h.handler.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("owner %s: %d %s", path, response.Code, response.Body)
		}
		if path == "/owner/capsules" {
			body := response.Body.String()
			if !strings.Contains(body, `<option value="tlock">`) || strings.Index(body, "plaintext through the server") > strings.Index(body, `<form`) {
				t.Fatal("tlock selection or pre-upload disclosure is absent")
			}
			for _, text := range []string{"Web preparation passes plaintext through the server", "server operator", "outage", "public data", "4 bytes", "tlock", "Create draft", "Publish capsule"} {
				if !strings.Contains(body, text) {
					t.Fatalf("missing %q", text)
				}
			}
			if !strings.Contains(response.Header().Get("Content-Security-Policy"), "script-src 'self'") {
				t.Fatal("script policy")
			}
		}
	}
}
