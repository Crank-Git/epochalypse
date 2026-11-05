package owner

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCSRFProtectsEveryOwnerMutation(t *testing.T) {
	s := sessionStore(t)
	session, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path string }{
		{"POST", "/api/v1/session"}, {"POST", "/owner/sign-in"},
		{"DELETE", "/api/v1/session"}, {"POST", "/owner/sign-out"},
		{"POST", "/api/v1/tokens"}, {"DELETE", "/api/v1/tokens/" + strings.Repeat("a", 32)},
	} {
		for _, proof := range []string{"missing", "wrong", "query", "no-cookie", "duplicate-cookie", "duplicate-header", "other-browser"} {
			t.Run(route.path+"/"+proof, func(t *testing.T) {
				r := httptest.NewRequest(route.method, "https://localhost"+route.path, nil)
				r.AddCookie(&http.Cookie{Name: cookieName, Value: session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
				addCSRF(t, s, r)
				alterCSRFProof(t, s, r, proof, session)
				w := httptest.NewRecorder()
				s.Handler(false).ServeHTTP(w, r)
				if w.Code != 403 || !strings.Contains(w.Body.String(), "invalid_csrf") || strings.Contains(w.Body.String(), session) {
					t.Fatalf("CSRF rejection: %d %s", w.Code, w.Body)
				}
				if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" || w.Header().Get("Content-Security-Policy") == "" {
					t.Fatal("rejection lacks response safeguards")
				}
			})
		}
	}
	if s.login.identity.count != 0 {
		t.Fatal("CSRF rejections consume password attempts")
	}
}

func TestCSRFRejectsForeignOriginsWithValidProof(t *testing.T) {
	s := sessionStore(t)
	for _, origin := range []string{"https://foreign.example", "http://localhost", "null", "https://localhost:443", "https://localhost/", "https://localhost?", "https://localhost#fragment", "https://user@localhost", "https://localhost https://foreign.example", ":"} {
		r := httptest.NewRequest("POST", "https://localhost/api/v1/session", strings.NewReader(`{"password":"synthetic"}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		addCSRF(t, s, r)
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("origin %q: %d", origin, w.Code)
		}
	}
	for _, headers := range []http.Header{
		{"Origin": {"https://localhost", "https://localhost"}},
		{"Sec-Fetch-Site": {"cross-site"}},
	} {
		r := httptest.NewRequest("POST", "https://localhost/api/v1/session", nil)
		r.Header = headers
		addCSRF(t, s, r)
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("foreign browser request passes")
		}
	}
}

func TestCSRFBootstrapReusesAProtectedRandomCookie(t *testing.T) {
	s := sessionStore(t)
	r := httptest.NewRequest("GET", "https://localhost/api/v1/csrf", nil)
	addCSRF(t, s, r)
	cookie := r.CookiesNamed(csrfCookieName)[0]
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(raw) != 32 {
		t.Fatal("token lacks 256 random bits")
	}
	for _, path := range []string{"/api/v1/csrf", "/owner"} {
		r.URL.Path = path
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != 200 || len(w.Result().Cookies()) != 0 || !strings.Contains(w.Body.String(), cookie.Value) {
			t.Fatalf("bootstrap does not reuse cookie: %d %s", w.Code, w.Body)
		}
	}
	for _, existing := range []string{"", "short", "duplicate"} {
		r := httptest.NewRequest("GET", "https://localhost/api/v1/csrf", nil)
		if existing != "" {
			r.AddCookie(&http.Cookie{Name: csrfCookieName, Value: existing, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
		if existing == "duplicate" {
			r.AddCookie(&http.Cookie{Name: csrfCookieName, Value: existing, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, r)
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" || cookies[0].Domain != "" {
			t.Fatal("bootstrap cookie lacks protection")
		}
	}
}

func TestCSRFFormProofUsesOnlyOneBoundedBodyField(t *testing.T) {
	s := sessionStore(t)
	for _, proof := range []string{"valid", "missing", "wrong", "duplicate", "malformed", "oversized"} {
		r := httptest.NewRequest("POST", "https://localhost/owner/sign-in", nil)
		addCSRF(t, s, r)
		token := r.Header.Get("X-CSRF-Token")
		r.Header.Del("X-CSRF-Token")
		form := url.Values{"password": {"wrong"}, "csrf_token": {token}}
		switch proof {
		case "missing":
			form.Del("csrf_token")
		case "wrong":
			form.Set("csrf_token", "wrong")
		case "duplicate":
			form.Add("csrf_token", token)
		case "oversized":
			form.Set("password", strings.Repeat("x", 8193))
		}
		body := form.Encode()
		if proof == "malformed" {
			body = "%zz"
		}
		r.Body = http.NoBody
		if body != "" {
			r.Body = io.NopCloser(strings.NewReader(body))
		}
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "https://localhost")
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, r)
		want := 403
		if proof == "valid" {
			want = 401
		}
		if w.Code != want {
			t.Fatalf("form %s: %d %s", proof, w.Code, w.Body)
		}
	}
}

func TestScopeExemptsOnlyValidatedBearerMutations(t *testing.T) {
	s := sessionStore(t)
	grant, err := s.CreateToken(TokenRequest{"automation", []string{"switch:checkin"}, time.Now().Add(time.Hour)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		bearer, scope string
		proof         bool
		want          int
	}{
		{grant.Token, "switch:checkin", false, 204},
		{grant.Token, "switch:write", false, 403},
		{"invalid", "switch:checkin", false, 401},
		{"", "switch:checkin", false, 403},
		{"", "switch:checkin", true, 204},
	} {
		r := httptest.NewRequest("POST", "https://localhost/resource", nil)
		r.AddCookie(&http.Cookie{Name: cookieName, Value: session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		if test.bearer != "" {
			r.Header.Set("Authorization", "Bearer "+test.bearer)
		}
		if test.proof {
			addCSRF(t, s, r)
		}
		w := httptest.NewRecorder()
		s.RequireScope(test.scope, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
		if w.Code != test.want {
			t.Fatalf("bearer %t, proof %t: %d", test.bearer != "", test.proof, w.Code)
		}
	}
}

func TestSignOutRejectsExplicitAuthorization(t *testing.T) {
	s := sessionStore(t)
	session, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path string }{{"POST", "/owner/sign-out"}, {"DELETE", "/api/v1/session"}} {
		r := httptest.NewRequest(route.method, "https://localhost"+route.path, nil)
		r.AddCookie(&http.Cookie{Name: cookieName, Value: session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		r.Header.Set("Authorization", "Bearer invalid")
		addCSRF(t, s, r)
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("explicit bearer falls back to session")
		}
		if _, err := s.Authenticate(session, time.Now()); err != nil {
			t.Fatal("rejected sign-out revokes session")
		}
	}
}

func alterCSRFProof(t *testing.T, s *Store, r *http.Request, proof, session string) {
	t.Helper()
	switch proof {
	case "missing":
		r.Header.Del("X-CSRF-Token")
	case "wrong":
		r.Header.Set("X-CSRF-Token", strings.Repeat("x", 43))
	case "query":
		r.URL.RawQuery = "csrf_token=" + r.Header.Get("X-CSRF-Token")
		r.Header.Del("X-CSRF-Token")
	case "no-cookie":
		r.Header.Set("Cookie", cookieName+"="+session)
	case "duplicate-cookie":
		r.AddCookie(r.CookiesNamed(csrfCookieName)[0])
	case "duplicate-header":
		r.Header.Add("X-CSRF-Token", r.Header.Get("X-CSRF-Token"))
	case "other-browser":
		other := httptest.NewRequest("POST", "/", nil)
		addCSRF(t, s, other)
		r.Header.Set("X-CSRF-Token", other.Header.Get("X-CSRF-Token"))
	}
}
