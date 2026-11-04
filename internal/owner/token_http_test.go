package owner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScopedRequestsRejectEscalationAndRevokedTokens(t *testing.T) {
	s := sessionStore(t)
	grant, err := s.CreateToken(TokenRequest{"automation", []string{"switch:checkin"}, time.Now().Add(time.Hour)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	for _, test := range []struct {
		scope, header string
		cookie        bool
		status        int
	}{
		{"switch:checkin", "Bearer " + grant.Token, false, 204},
		{"switch:checkin", "bearer " + grant.Token, false, 204},
		{"switch:checkin", "Bearer   " + grant.Token, false, 204},
		{"switch:write", "Bearer " + grant.Token, false, 403},
		{"read", "Bearer invalid", true, 401},
		{"read", "Basic invalid", false, 401},
		{"read", "Bearer  invalid", false, 401},
		{"read", "", false, 401},
		{"read", "", true, 204},
		{"admin", "", true, 403},
	} {
		r := httptest.NewRequest("GET", "/resource", nil)
		if test.header != "" {
			r.Header.Set("Authorization", test.header)
		}
		if test.cookie {
			r.AddCookie(&http.Cookie{Name: cookieName, Value: session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
		w := httptest.NewRecorder()
		s.RequireScope(test.scope, next).ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("scope %s: %d %s", test.scope, w.Code, w.Body)
		}
		if w.Code >= 400 && strings.Contains(w.Body.String(), grant.Token) {
			t.Fatal("error exposes token")
		}
	}
	r := httptest.NewRequest("GET", "/resource", nil)
	r.Header.Add("Authorization", "Bearer "+grant.Token)
	r.Header.Add("Authorization", "Bearer "+grant.Token)
	w := httptest.NewRecorder()
	s.RequireScope("switch:checkin", next).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("duplicate authorization accepted")
	}
	for _, method := range []string{"POST", "DELETE"} {
		r := httptest.NewRequest(method, "/api/v1/tokens/"+grant.ID, nil)
		if method == "POST" {
			r.URL.Path = "/api/v1/tokens"
		}
		r.Header.Set("Authorization", "Bearer "+grant.Token)
		r.AddCookie(&http.Cookie{Name: cookieName, Value: session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		w := httptest.NewRecorder()
		s.Handler(true).ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("API token administers tokens")
		}
	}
	if err := s.RevokeToken(grant.ID); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("GET", "/resource", nil)
	r.Header.Set("Authorization", "Bearer "+grant.Token)
	w = httptest.NewRecorder()
	s.RequireScope("switch:checkin", next).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("revoked token reaches handler")
	}
}

func TestTokenAPIRejectsInvalidBodies(t *testing.T) {
	s := sessionStore(t)
	session, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		body, media string
		status      int
		chunked     bool
	}{
		{`{`, "application/json", 400, false},
		{`{}`, "application/json", 400, false},
		{`{"unknown":true}`, "application/json", 400, false},
		{`{} {}`, "application/json", 400, false},
		{`{}`, "text/plain", 400, false},
		{`{"name":"test","scopes":["admin"],"expires_at":"2099-01-01T00:00:00Z"}`, "application/json", 400, false},
		{strings.Repeat("x", 8193), "application/json", 413, false},
		{`{"name":"` + strings.Repeat("x", 8193) + `"}`, "application/json", 413, true},
		{`{"name":"test","scopes":["read"],"expires_at":"2099-01-01T00:00:00Z"}` + strings.Repeat(" ", 8193), "application/json", 413, true},
	} {
		r := httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader(test.body))
		if test.chunked {
			r.ContentLength = -1
		}
		r.Header.Set("Content-Type", test.media)
		r.AddCookie(&http.Cookie{Name: cookieName, Value: session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		w := httptest.NewRecorder()
		s.Handler(true).ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("invalid body: %d %s", w.Code, w.Body)
		}
		if w.Header().Get("X-Request-ID") == "" || !strings.Contains(w.Body.String(), w.Header().Get("X-Request-ID")) {
			t.Fatal("error lacks request id")
		}
	}
	r := httptest.NewRequest("DELETE", "/api/v1/tokens/bad", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w := httptest.NewRecorder()
	s.Handler(true).ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("invalid id revokes token")
	}
}

func TestTokenAPIReportsUnavailableState(t *testing.T) {
	s := sessionStore(t)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, handler := range []http.Handler{s.RequireOwner(http.HandlerFunc(s.createToken)), s.RequireScope("read", http.HandlerFunc(s.createToken))} {
		r := httptest.NewRequest("POST", "/api/v1/tokens", nil)
		r.AddCookie(&http.Cookie{Name: cookieName, Value: strings.Repeat("A", 43), Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 503 {
			t.Fatalf("closed state: %d", w.Code)
		}
	}
	for _, method := range []string{"POST", "DELETE"} {
		r := httptest.NewRequest(method, "/api/v1/tokens", strings.NewReader(`{"name":"test","scopes":["read"],"expires_at":"2099-01-01T00:00:00Z"}`))
		r.Header.Set("Content-Type", "application/json")
		r.SetPathValue("id", strings.Repeat("a", 32))
		w := httptest.NewRecorder()
		if method == "POST" {
			s.createToken(w, r)
		} else {
			s.revokeToken(w, r)
		}
		if w.Code != 503 {
			t.Fatalf("closed mutation: %d", w.Code)
		}
	}
}

func TestTokenAPIRequiresOwnerAndReturnsOneTimeValue(t *testing.T) {
	s := sessionStore(t)
	session, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body := `{"name":"automation","scopes":["read"],"expires_at":"2099-01-01T00:00:00Z"}`
	request := func(method, path, body, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: cookieName, Value: cookie, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
		w := httptest.NewRecorder()
		s.Handler(true).ServeHTTP(w, r)
		return w
	}
	if w := request("POST", "/api/v1/tokens", body, ""); w.Code != 401 {
		t.Fatalf("unauthenticated creation: %d", w.Code)
	}
	w := request("POST", "/api/v1/tokens", body, session)
	if w.Code != 201 {
		t.Fatalf("creation: %d %s", w.Code, w.Body)
	}
	var result struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.ID) != 32 || len(result.Token) != 43 {
		t.Fatalf("invalid result: id=%d token=%d", len(result.ID), len(result.Token))
	}
	if w := request("GET", "/api/v1/tokens/"+result.ID, "", session); strings.Contains(w.Body.String(), result.Token) {
		t.Fatal("token value appears again")
	}
	for range 2 {
		if w := request("DELETE", "/api/v1/tokens/"+result.ID, "", session); w.Code != 204 || w.Body.Len() != 0 {
			t.Fatalf("revocation: %d %s", w.Code, w.Body)
		}
	}
}
