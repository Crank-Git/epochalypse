package owner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebRejectsMutationsWithoutCSRF(t *testing.T) {
	s := sessionStore(t)
	r := httptest.NewRequest("POST", "https://localhost/api/v1/session", strings.NewReader(`{"password":"synthetic"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler(false).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("missing CSRF token: got %d, want 403", w.Code)
	}
}

func TestWebLimitsFailedSignIns(t *testing.T) {
	s := sessionStore(t)
	h := s.Handler(false)
	for attempt := 1; attempt <= 6; attempt++ {
		r := httptest.NewRequest("POST", "https://localhost/api/v1/session", strings.NewReader(`{"password":"wrong"}`))
		r.Header.Set("Content-Type", "application/json")
		addCSRF(t, s, r)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := 401
		if attempt == 6 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d: got %d, want %d", attempt, w.Code, want)
		}
	}
}

func addCSRF(t *testing.T, s *Store, r *http.Request) {
	t.Helper()
	w := httptest.NewRecorder()
	s.Handler(true).ServeHTTP(w, httptest.NewRequest("GET", "https://localhost/api/v1/csrf", nil))
	var body struct {
		Token string `json:"csrf_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(body.Token) != 43 || len(w.Result().Cookies()) != 1 {
		t.Fatal("CSRF bootstrap fails")
	}
	r.AddCookie(w.Result().Cookies()[0])
	r.Header.Set("X-CSRF-Token", body.Token)
}
