package owner

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebSignsInAndRevokesTheSession(t *testing.T) {
	s := sessionStore(t)
	h := s.Handler(false)
	request := httptest.NewRequest("POST", "https://localhost/api/v1/session", strings.NewReader(`{"password":"synthetic"}`))
	request.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	addCSRF(t, s, request)
	h.ServeHTTP(w, request)
	if w.Code != 204 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("sign in: %d %s", w.Code, w.Body)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" {
		t.Fatalf("unsafe cookie: %+v", cookie)
	}
	for _, step := range []struct {
		method, path string
		code         int
		contains     string
	}{
		{"GET", "/owner", 200, "Signed in"},
		{"DELETE", "/api/v1/session", 204, ""},
		{"GET", "/owner", 200, "Owner sign-in"},
	} {
		r := httptest.NewRequest(step.method, "https://localhost"+step.path, nil)
		r.AddCookie(cookie)
		w = httptest.NewRecorder()
		addCSRF(t, s, r)
		h.ServeHTTP(w, r)
		if w.Code != step.code || !strings.Contains(w.Body.String(), step.contains) {
			t.Fatalf("%s: %d %s", step.path, w.Code, w.Body)
		}
		if step.method == "DELETE" && (len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge >= 0) {
			t.Fatal("sign out retains cookie")
		}
	}
}

func TestWebRejectsMalformedCredentials(t *testing.T) {
	s := sessionStore(t)
	for _, body := range []string{`bad`, `{}`, `{"password":1}`, `{"extra":true}`, `{"password":"synthetic"} {}`, strings.Repeat("x", 8193)} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "https://localhost/api/v1/session", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		addCSRF(t, s, r)
		s.Handler(false).ServeHTTP(w, r)
		expected := 400
		if body == `{}` {
			expected = 401
		}
		if len(body) > 8192 {
			expected = 413
		}
		if w.Code != expected {
			t.Fatalf("body %q: %d", body[:min(len(body), 40)], w.Code)
		}
	}
	for _, media := range []string{"", "text/plain", "application/json; broken"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "https://localhost/api/v1/session", strings.NewReader(`{"password":"synthetic"}`))
		r.Header.Set("Content-Type", media)
		addCSRF(t, s, r)
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "https://localhost/owner/sign-in", strings.NewReader("password=%zz"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addCSRF(t, s, r)
	s.Handler(false).ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestWebFormsRedirectAfterSignInAndSignOut(t *testing.T) {
	s := sessionStore(t)
	var cookie *http.Cookie
	for _, step := range []struct {
		path, password string
		code           int
	}{
		{"/owner/sign-in", "wrong", 401}, {"/owner/sign-in", "synthetic", 303}, {"/owner/sign-out", "", 303},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "https://localhost"+step.path, strings.NewReader("password="+step.password))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		addCSRF(t, s, r)
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != step.code {
			t.Fatalf("%s: %d %s", step.path, w.Code, w.Body)
		}
		if step.code == 401 {
			if !strings.Contains(w.Body.String(), `role="alert"`) || strings.Contains(w.Body.String(), "wrong") {
				t.Fatal("unsafe form error")
			}
		} else {
			if w.Header().Get("Location") != "/owner" {
				t.Fatal("missing redirect")
			}
			cookie = w.Result().Cookies()[0]
		}
	}
}

func TestWebRejectsInvalidAndExpiredSessions(t *testing.T) {
	s := sessionStore(t)
	token, err := s.SignIn([]byte("synthetic"), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "invalid", token} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "https://localhost/owner", nil)
		if value != "" {
			r.Header.Set("Cookie", cookieName+"="+value)
		}
		addCSRF(t, s, r)
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != 200 || strings.Contains(w.Body.String(), "Signed in") {
			t.Fatal("invalid session permits access")
		}
	}
	for _, value := range []string{"", "invalid"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "https://localhost/api/v1/session", nil)
		if value != "" {
			r.Header.Set("Cookie", cookieName+"="+value)
		}
		addCSRF(t, s, r)
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
}

func TestWebFailsClosedWhenTheStoreCloses(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		path := "/api/v1/session"
		if method == "GET" {
			path = "/owner"
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, "https://localhost"+path, strings.NewReader(`{"password":"synthetic"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Cookie", cookieName+"="+strings.Repeat("A", 43))
		addCSRF(t, s, r)
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != 503 || strings.Contains(w.Body.String(), "synthetic") {
			t.Fatalf("closed store: %d %s", w.Code, w.Body)
		}
	}
}

func TestWebRejectsPlainHTTPOutsideTheLab(t *testing.T) {
	s := sessionStore(t)
	for _, path := range []string{"/owner", "/api/v1/session", "/owner/sign-in", "/owner/sign-out"} {
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, httptest.NewRequest("POST", "http://localhost"+path, nil))
		if w.Code != http.StatusUpgradeRequired {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.Handler(true).ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/owner", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestWebUsesGenericCredentialErrors(t *testing.T) {
	s := sessionStore(t)
	for _, body := range []string{`{"password":"wrong-secret"}`, `{"password":""}`} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "https://localhost/api/v1/session", strings.NewReader(body))
		r.TLS = &tls.ConnectionState{}
		r.Header.Set("Content-Type", "application/json")
		addCSRF(t, s, r)
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != 401 || !strings.Contains(w.Body.String(), `"code":"invalid_credentials"`) || strings.Contains(w.Body.String(), "wrong-secret") || len(w.Result().Cookies()) != 0 {
			t.Fatalf("credentials: %d %s", w.Code, w.Body)
		}
	}
}

func TestWebCorrelatesErrorsWithARequestID(t *testing.T) {
	s := sessionStore(t)
	previous := ""
	for range 2 {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "https://localhost/api/v1/session", strings.NewReader(`{"password":"wrong"}`))
		r.Header.Set("Content-Type", "application/json")
		addCSRF(t, s, r)
		s.Handler(false).ServeHTTP(w, r)
		var body struct {
			Error struct {
				RequestID string `json:"request_id"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		id := body.Error.RequestID
		if id == "" || id == previous || id != w.Header().Get("X-Request-ID") {
			t.Fatal("error has no unique request ID")
		}
		previous = id
	}
}
