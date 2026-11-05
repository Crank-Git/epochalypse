package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/owner"
	"time"
)

func TestCapsuleDeleteRequiresConfirmationAndAuthorization(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "server-held")
	grant, err := h.state.owner.CreateToken(owner.TokenRequest{Name: "Read only", Scopes: []string{"read"}, ExpiresAt: time.Now().Add(time.Hour)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body, bearer string
		session, csrf      bool
		want               int
	}{
		{"anonymous", `{"confirm":"` + manifest.ID + `"}`, "", false, false, 401},
		{"wrong scope", `{"confirm":"` + manifest.ID + `"}`, grant.Token, false, false, 403},
		{"no CSRF", `{"confirm":"` + manifest.ID + `"}`, "", true, false, 403},
		{"missing confirmation", `{}`, h.bearer, false, false, 400},
		{"wrong confirmation", `{"confirm":"yes"}`, h.bearer, false, false, 400},
		{"extra field", `{"confirm":"` + manifest.ID + `","extra":true}`, h.bearer, false, false, 400},
		{"trailing body", `{"confirm":"` + manifest.ID + `"}{}`, h.bearer, false, false, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("DELETE", "https://localhost/api/v1/capsules/"+manifest.ID, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			if tc.bearer != "" {
				r.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			if tc.session {
				r.AddCookie(&http.Cookie{Name: "__Host-epochalypse-session", Value: h.session, Secure: true})
			}
			w := h.send(r)
			if w.Code != tc.want {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if _, err := h.store.Get(manifest.ID); err != nil {
				t.Fatal("failed deletion changes state", err)
			}
		})
	}
	r := httptest.NewRequest("DELETE", "https://localhost/api/v1/capsules/"+manifest.ID, strings.NewReader(`{"confirm":"`+manifest.ID+`"}`))
	r.Header.Set("Authorization", "Bearer "+h.bearer)
	r.Header.Set("Content-Type", "application/json")
	if w := h.send(r); w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if _, err := h.store.Get(manifest.ID); err == nil {
		t.Fatal("local record remains")
	}
}
